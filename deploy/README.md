# Kubernetes / GKE へのデプロイ

既存の Compose と同じ control・agent・PostgreSQL 16・Redis 7 を動かす。
`base/` が共通定義、`overlays/gke/` が GKE、`overlays/k8s/` がそれ以外の Kubernetes 用。
以下のコマンドは repo root で実行する。ファイルを作った時点ではクラスタへの適用は行わない。

## 構成

| 対象 | 定義 |
|---|---|
| control | Deployment、ClusterIP Service、起動・readiness・liveness probe |
| agent | Deployment、control の起動待ち、専用 ServiceAccount |
| PostgreSQL | StatefulSet、10 GiB PVC、専用の初期化イメージ |
| Redis | StatefulSet、1 GiB PVC、AOF |
| 設定 | ConfigMap。アプリ・DB の資格情報は用途別の3つの Secret |
| GKE の GCS 認証 | Workload Identity Federation、control / agent 別の IAM service account |
| GKE の公開 | cluster 内の cloudflared（Cloudflare Tunnel、2 replica + PDB）→ control Service の80番。外部 LB・固定 IP は使わない |
| 通信 | DB・Redis への ingress は同 namespace の control / agent のみ。agent への ingress は閉じる。GKE では control への ingress も cloudflared / agent のみ、cloudflared への ingress は閉じる |

単一 replica の小規模構成。DB / Redis は高可用構成ではなく、バックアップは別途必要。
control / agent は `Recreate` で更新時に短い停止がある。
agent には HTTP health endpoint がないので架空の probe は設定しない。プロセス異常終了は Kubernetes が再起動する。
control の readiness は DB と Redis の疎通、liveness は HTTP port の応答で判定する。

**初期状態は control / agent が0 replica。** DB の初期化・データ移行前に自動通知が動かないようにしている。
準備が済んだら利用する overlay の `replicas` の `count` を両方 `1` にする。その後の適用でも `1` を維持する。

## 1. 接続先と設定

用意するもの：kubectl（Kustomize 付き）、Docker buildx、python3、公開先 registry、永続ボリュームに対応した Kubernetes cluster。
GKE では gcloud、VPC-native cluster、Persistent Disk CSI driver、Workload Identity Federation を利用する。
NetworkPolicy が効く CNI（GKE Dataplane V2 等）を使う。GKE Standard ではアプリを配置する node pool も `GKE_METADATA` を有効にする。

```sh
kubectl config get-contexts
export KUBE_CONTEXT='対象クラスタのcontext名'
export OVERLAY='deploy/overlays/gke' # 汎用 Kubernetes は deploy/overlays/k8s
```

編集箇所：

- `$OVERLAY/kustomization.yaml`：3つの image の registry / tag。
- `$OVERLAY/settings.yaml`：GCS bucket、署名用 service account、初期管理者 `APP_USERNAME`。
- GKE は同ファイルの control / agent の IAM service account annotation。
- GKE の公開 hostname は Cloudflare の Tunnel の route 側で決める（manifest には書かない）。
- GKE の `standard-rwo`、汎用 Kubernetes の default StorageClass が実在することを確認する。
- 既存データを移す場合、`APP_USERNAME` は現在のユーザー名に合わせる。

`base/config.yaml` は共通値。モデルは現在の Compose と同じものを引き継いでいる。
`DB_NAME` を変更する場合は PostgreSQL 側の `POSTGRES_DB` も揃える。
DB role 名 `suyasuya_control` / `suyasuya_agent` は schema と一致させ、変更しない。

## 2. イメージ

```sh
export PROJECT_ID='既存のGCPプロジェクトID'
export REGION='asia-northeast1'
export REGISTRY="$REGION-docker.pkg.dev/$PROJECT_ID/suyasuya"
export TAG='deploy-20260929-1' # 内容ごとに新しい tag にする

# Artifact Registry の repository は事前に用意する。
gcloud auth configure-docker "$REGION-docker.pkg.dev"
./deploy/scripts/build-images.sh "$REGISTRY" "$TAG" --push
```

`--push` を省くと local build のみ。標準は linux/amd64、別 architecture は `PLATFORM` で指定する。
control・agent は既存 Dockerfile、postgres は `docker/postgres.Dockerfile` を使う。
DB image は repo の `schema.sql` と `docker/20-roles.sh` を取り込むので SQL を複製管理しない。

overlay の3つの `newName` / `newTag` をこの出力に揃える。
GKE の image pull は node の service account が行うため、registry に対する `roles/artifactregistry.reader` が必要。
アプリ用 Workload Identity の権限だけでは image pull はできない。
汎用 Kubernetes の private registry では別途 imagePullSecret を作り、4 workload の Pod template に `imagePullSecrets` を指定する。

## 3. Secret

```sh
umask 077
cp deploy/secrets/postgres.env.example deploy/secrets/postgres.env
cp deploy/secrets/control.env.example deploy/secrets/control.env
cp deploy/secrets/agent.env.example deploy/secrets/agent.env
```

3ファイルを編集する。GKE では Tunnel の token を入れる `cloudflared.env` も作る（下）。`KEY=値` をそのまま記載し、値を引用符で囲まない。local `.env` 全体を渡さない。
`deploy/secrets/` は例以外を Git 対象外にしている。

- `postgres.env`：管理者 password と、control / agent の DB password。
- `control.env`：control の DB password と初期管理者の `APP_PASSWORD`。
- `agent.env`：agent の DB password と `GEMINI_API_KEY`。
- 同じ role の DB password はファイル間で一致させる。
- `cloudflared.env`（GKE のみ）：`TUNNEL_TOKEN=` の1行。Zero Trust の Tunnel 作成画面の `--token` の値。無ければ script は飛ばす。
- 既存ユーザーの login password は DB に保存されている。`APP_PASSWORD` の変更だけでは既存ユーザーの password は変わらない。

```sh
./deploy/scripts/apply-secrets.sh "$KUBE_CONTEXT"
```

この script は不足・DB password の不一致を確認し、値を表示せずに Secret を適用する。
Secret 更新は Pod の環境変数に自動反映されない。変更後は対象 Deployment を再起動する。
DB の role password は空の DB 初期化時だけ設定されるため、稼働後の変更には DB 側の `ALTER ROLE` も必要。

## 4. GKE の GCS 認証・HTTPS

詳細コマンドは [GKE 設定](gke.md)。control / agent の IAM service account を分ける。
control を signed URL の signer とし、bucket に object 作成・読み取り、自己の `signBlob` 権限を持たせる。
agent は bucket の object 読み取りだけを許可する。
GKE overlay は JSON key を mount せず、ADC が GKE metadata server を使う。

公開は cluster 内の cloudflared（`overlays/gke/cloudflared.yaml`）が受け持つ。Tunnel は remote-managed で、
route と DNS は Cloudflare の UI で設定する（2026-10-10 に GCE Ingress から移行。外部 LB の転送ルールの費用を削るため）。

1. Zero Trust → Networks → Tunnels で Tunnel を作り、token を `deploy/secrets/cloudflared.env` に書いて `apply-secrets.sh` を流す
2. overlay を apply し、Tunnel が HEALTHY になるのを確認する
3. Public Hostname に公開 hostname → `HTTP` / `control:80` を足す（同名の DNS record が既にあれば先に消す）

TLS は Cloudflare の edge で終端し、cloudflared → control は cluster 内の HTTP。
control は `X-Forwarded-Proto` を見て session cookie に Secure を付ける。CF Access は掛けていない（審査員が使うため。アプリの login で守る）。

`deploy/examples/gcs-cors.json` の origin を本番 URL に置き換える。
既存 bucket の CORS は先に確認し、開発用 origin も必要なら同じファイルに残す。
次のコマンドは bucket の CORS 全体を置き換える。

```sh
gcloud storage buckets update "gs://実際のbucket名" --cors-file=deploy/examples/gcs-cors.json
```

## 5. 汎用 Kubernetes の場合

`overlays/k8s/` は各 Pod に ADC の Secret を read-only mount する。
署名用 account・bucket の権限は GKE と同じ。control と agent の認証は別々に用意する。

```sh
kubectl --context "$KUBE_CONTEXT" -n suyasuya create secret generic control-gcp-credentials \
  --from-file=credentials.json=/path/to/control-credentials.json
kubectl --context "$KUBE_CONTEXT" -n suyasuya create secret generic agent-gcp-credentials \
  --from-file=credentials.json=/path/to/agent-credentials.json
```

公開にはクラスタの Ingress controller と TLS 証明書を用意する。
`deploy/examples/ingress.yaml` の class・host・TLS Secret 名を変更し、別途 apply する。
HTTP から HTTPS への redirect は使う controller 側で設定する。
外部の HTTPS proxy / Tunnel を使う場合も、接続先は control Service の80番。
DB・Redis は外部に公開しない。`APP_SECURE_COOKIE=true` なのでログインには HTTPS を使う。

## 6. 展開確認とストレージ起動

```sh
kubectl kustomize "$OVERLAY" > /tmp/suyasuya-deploy.yaml
# 展開結果に example / replace-me が残っていないことを確認する。
kubectl --context "$KUBE_CONTEXT" apply --dry-run=server -f /tmp/suyasuya-deploy.yaml
kubectl --context "$KUBE_CONTEXT" apply -f /tmp/suyasuya-deploy.yaml
kubectl --context "$KUBE_CONTEXT" -n suyasuya rollout status statefulset/postgres --timeout=300s
kubectl --context "$KUBE_CONTEXT" -n suyasuya rollout status statefulset/redis --timeout=300s
```

PostgreSQL の最初の起動で schema と role password を設定する。PVC は Pod の再作成でも残る。
初期化 SQL は空の PGDATA でのみ実行される。イメージ更新だけで既存 DB の schema は変わらない。
既存 DB 更新はバックアップ後、対象の migration SQL を管理者で明示的に適用する。
最新 schema で新規作成した DB に過去の migration を重ねて実行しない。
PVC の StorageClass 等は後から StatefulSet の更新では変更できないため、最初に確定する。

## 7. Compose のデータを移す場合

**初回移行先の空の DB にのみ行う。** control / agent はまだ0 replica のままにする。
source の schema がこの repo の schema と同じ版であることを確認する。
稼働済みの移行先へ再適用すると、以後に追加されたデータを失う。

```sh
# local の更新・通知を止め、DB / Redis は稼働させたまま dump を取得する。
docker compose stop control agent
umask 077
mkdir -p tmp/migration
docker compose exec -T postgres sh -ec 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' \
  > tmp/migration/compose.dump

# 移行先にユーザーがまだ存在しないことを確認する。0 以外なら restore を進めない。
kubectl --context "$KUBE_CONTEXT" -n suyasuya exec postgres-0 -- \
  sh -ec 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "SELECT count(*) FROM users"'

# 空の移行先にある初期 schema を、dump 内の schema・権限・データに置き換える。
kubectl --context "$KUBE_CONTEXT" -n suyasuya exec -i postgres-0 -- \
  sh -ec 'pg_restore --exit-on-error --single-transaction --clean --if-exists --no-owner -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
  < tmp/migration/compose.dump
```

`pg_dump` は role 定義や password を含まない。移行先の role / password は初期化時に設定したものを使う。
ユーザー・睡眠記録・PVT・個人 prompt・設定・job・通知履歴は DB の移行で引き継ぐ。
GCS は同じ bucket を使えば画像をコピーする必要がない。
Redis の内容は移さず、DB の未処理 job を control の再投入処理で回復する。

## 8. アプリを起動する

新規利用なら初期化後、移行なら restore と件数確認後に、overlay の `replicas` の `count` を両方 `1` に変更する。

```sh
kubectl --context "$KUBE_CONTEXT" apply -k "$OVERLAY"
kubectl --context "$KUBE_CONTEXT" -n suyasuya rollout status deployment/control --timeout=300s
kubectl --context "$KUBE_CONTEXT" -n suyasuya rollout status deployment/agent --timeout=300s
kubectl --context "$KUBE_CONTEXT" -n suyasuya get pods,pvc,svc
kubectl --context "$KUBE_CONTEXT" -n suyasuya logs deployment/agent --tail=30
```

GKE は `kubectl --context "$KUBE_CONTEXT" -n suyasuya logs deployment/cloudflared` で `Registered tunnel connection` を確認する。
HTTPS のログイン、記録一覧、画像 PUT → OCR、PVT、設定保存を確認する。
定時処理は保存済み設定で再開する。local と GKE の control / agent を同時に動かすと重複通知につながるので、切り替え後は local 側を停止したままにする。

戻す場合はまず GKE 側の control / agent を0 replica にする。移行後に GKE に追加したデータがあれば、旧 local をそのまま起動せず、逆方向の DB 移行も行う。

## 更新とバックアップ

更新は新しい image tag で build / push し、overlay を変更して apply する。
アプリだけを戻す場合も、schema と互換性のある image tag を選ぶ。

```sh
umask 077
mkdir -p tmp/backups
kubectl --context "$KUBE_CONTEXT" -n suyasuya exec postgres-0 -- \
  sh -ec 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' \
  > "tmp/backups/suyasuya-$(date +%Y%m%d-%H%M%S).dump"
```

dump には個人データ・送信先が含まれるので Git に入れず、必要に応じて別の保管先へ退避する。
通常の停止は control / agent の replica を0にする。namespace や PVC の削除はデータ削除を伴うため停止用途には使わない。

## 参考

- [Workload Identity Federation for GKE](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/workload-identity)
- [Cloudflare Tunnel を Kubernetes で動かす](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/deployment-guides/kubernetes/)
