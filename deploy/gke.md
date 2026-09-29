# GKE の事前設定

以下は [デプロイ手順](README.md) の GKE 補足。既存 project・既存 bucket を使う。
cluster・Artifact Registry repository の作成は、この手順の前に済ませる。
ここに書かれた gcloud コマンドは実行すると IAM やクラウドリソースを変更する。

## 変数と API

```sh
export PROJECT_ID='GKEを置く既存project ID'
export BUCKET='既存の非公開bucket名'
export CONTROL_SA="suyasuya-control@$PROJECT_ID.iam.gserviceaccount.com"
export AGENT_SA="suyasuya-agent@$PROJECT_ID.iam.gserviceaccount.com"
export CLUSTER='cluster名'
export LOCATION='clusterのregionまたはzone'

gcloud services enable container.googleapis.com artifactregistry.googleapis.com \
  iamcredentials.googleapis.com storage.googleapis.com compute.googleapis.com \
  --project="$PROJECT_ID"
gcloud container clusters get-credentials "$CLUSTER" --location="$LOCATION" --project="$PROJECT_ID"
```

GKE Standard の場合は cluster の Workload Identity Federation と、利用する node pool の metadata server を有効にする。
Autopilot は Workload Identity Federation が有効。Standard の node pool に複数方式が混在する場合は、
control / agent の Pod に `nodeSelector: {iam.gke.io/gke-metadata-server-enabled: "true"}` を追加して配置先を固定する。
Autopilot にはその nodeSelector を追加しない。

```sh
# Standard のみ。クラスタ側の準備が済んでいれば再実行不要。
gcloud container clusters update "$CLUSTER" --location="$LOCATION" --project="$PROJECT_ID" \
  --workload-pool="$PROJECT_ID.svc.id.goog"
gcloud container node-pools update '利用するnode-pool名' --cluster="$CLUSTER" \
  --location="$LOCATION" --project="$PROJECT_ID" --workload-metadata=GKE_METADATA
```

VPC-native / HttpLoadBalancing / Persistent Disk CSI driver、NetworkPolicy を実施する CNI も必要。
node の service account には GKE の標準的な node 権限と Artifact Registry の読み取り権限を設定する。
既存の node pool を変更する際は、そのクラスタの他アプリへの影響も確認する。

## control / agent の認証

専用の IAM service account をまだ用意していなければ作る。

```sh
gcloud iam service-accounts create suyasuya-control --project="$PROJECT_ID"
gcloud iam service-accounts create suyasuya-agent --project="$PROJECT_ID"
```

GKE の Kubernetes ServiceAccount `suyasuya/control` と `suyasuya/agent` を対応する IAM service account に紐づける。

```sh
gcloud iam service-accounts add-iam-policy-binding "$CONTROL_SA" --project="$PROJECT_ID" \
  --role=roles/iam.workloadIdentityUser \
  --member="serviceAccount:$PROJECT_ID.svc.id.goog[suyasuya/control]"
gcloud iam service-accounts add-iam-policy-binding "$AGENT_SA" --project="$PROJECT_ID" \
  --role=roles/iam.workloadIdentityUser \
  --member="serviceAccount:$PROJECT_ID.svc.id.goog[suyasuya/agent]"
```

overlay の ServiceAccount annotation のメールアドレスも、この2つと一致させる。
これは JSON key を作らず、既存の ADC 対応コードを使う構成。

## bucket と署名

control を signed URL の signer とする。`GCS_SIGNER_SERVICE_ACCOUNT` は `$CONTROL_SA`。
API 呼び出しと署名のため、control の自己署名権限をその service account 上だけに付ける。

```sh
gcloud iam service-accounts add-iam-policy-binding "$CONTROL_SA" --project="$PROJECT_ID" \
  --role=roles/iam.serviceAccountTokenCreator --member="serviceAccount:$CONTROL_SA"
gcloud storage buckets add-iam-policy-binding "gs://$BUCKET" \
  --member="serviceAccount:$CONTROL_SA" --role=roles/storage.objectCreator
gcloud storage buckets add-iam-policy-binding "gs://$BUCKET" \
  --member="serviceAccount:$CONTROL_SA" --role=roles/storage.objectViewer
gcloud storage buckets add-iam-policy-binding "gs://$BUCKET" \
  --member="serviceAccount:$AGENT_SA" --role=roles/storage.objectViewer
```

bucket は非公開のままにする。browser は署名付き PUT / GET を使う。
CORS の origin を公開 URL に合わせる操作は [本体手順](README.md#4-gke-の-gcs-認証https) を参照。
既存の署名用 account を使う場合は、署名権限をその account に付け、署名者自身にも object 作成・読み取り権限を付ける。
Gemini API は今までの API key を Secret から渡す。GCS の認証とは別。

## Ingress の固定 IP と DNS

```sh
gcloud compute addresses create suyasuya-ip --global --project="$PROJECT_ID"
gcloud compute addresses describe suyasuya-ip --global --project="$PROJECT_ID" --format='value(address)'
```

その IP を公開ドメインの A record に設定し、GKE overlay の Ingress と ManagedCertificate の domain を揃える。
固定 IP 名を変更した場合は `kubernetes.io/ingress.global-static-ip-name` も変更する。
誤った AAAA record や別サービスへの proxy は証明書の検証に影響するので、DNS を直接 GKE の公開先へ向ける。

`BackendConfig` は `/healthz` の8080番をチェックし、`FrontendConfig` は HTTP を HTTPS に redirect する。
GKE Ingress は `kubernetes.io/ingress.class: gce` を使う。汎用 Kubernetes の `ingressClassName` と混ぜない。
LB と証明書を作るのは overlay 適用後。公開 URL で認証・画像 upload まで確認して切り替える。

## 公式資料

- [Workload Identity の有効化と service account の紐づけ](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/workload-identity)
- [BackendConfig / FrontendConfig](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/ingress-configuration)
- [ManagedCertificate と固定 IP](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/secure-traffic-management)
