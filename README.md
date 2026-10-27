# suyasuya

**定時に寝ることを支える agent。**

第 5 回 Agentic AI Hackathon with Google Cloud 応募作品。

## つくるもの

睡眠アプリの多くは、user に「つける」ことを求める。
だが**記録という行為そのものが、対象への注意になる**。
寝つきのように、意識するほど遠のく種類のものでは、それが逆に働くことがある。

だから suyasuya は **記録の手間を減らし、行動を支える。**

睡眠はデバイスが自動で測る。**screenshot を Gemini が読み取り、user が管理 UI で確認・修正して承認する（主要数値の手入力も可）。**
承認した記録の傾向の把握と、声のかけ方の判断は agent の側で担い、設定時刻に **Discord で催促する。**

介入先は睡眠時間ではなく**就寝時刻**に置いた。長さは意志で増やしにくいが、時刻は寄せられる。
スコアを毎朝突きつけて習慣化を迫る形は取らず、**agent が読んで、必要なときにだけ声をかける。**

| 機能 | 内容 |
|---|---|
| **記録** | screenshot を Gemini が読み取り、user が確認・承認する（主睡眠・昼寝、手入力も可） |
| **催促** | 設定時刻に agent が承認済み記録・PVT・通知履歴を tool で読み、送るか・何を伝えるかを判断して Discord に送る。見送ったときも理由を残す |
| **ふりかえり** | 睡眠時間・就寝時刻のグラフと、agent が書く週間レポート |
| **根拠（PVT）** | 1 分の反応時間テストで、主観だけでは気づきにくい日中の機能の変化を捉える |
| **可観測性** | agent が呼んだ tool・引数・結果・判断の理由を「agent ログ」で読み返せる |

agent の判断手順・目的・OCR の指示・人格（base prompt）はユーザーごとに PostgreSQL で版管理し、管理画面から再デプロイせずに調整できる。

## 技術スタック

- **Go** —— `control`（Web UI・API・scheduler）と `agent`（queue 処理・Reasoner loop）の 2 バイナリ構成。framework は使わず自前で実装
- **Gemini API（`gemini-3.8-flash`）** —— 画像の読み取り（OCR）・function calling による判断・文面生成
- **Google Cloud（GKE Autopilot / Cloud Storage）** —— 本番実行環境と画像保存（V4 署名付き URL による browser 直接 PUT）
- **PostgreSQL / Redis** —— 記録の正本と task queue（Redis は task ID のみ運ぶ）
- **Discord Webhook** —— 催促の出口（`Notifier` interface で分離）

## ディレクトリ構成

```text
cmd/
  control/       Web UI・API・scheduler・migration 管理
  agent/         Redis queue ワーカー・Gemini 呼び出し
internal/
  agent/         Reasoner（function-call loop）・tool registry・実行ログ記録
  service/       承認・記録管理・PVT 集計・OCR（internal/service/ocr）
  repository/    PostgreSQL クエリ・トランザクション（user_id 境界の強制）
  notifier/      Discord Webhook 配信・レート制限対応
  handler/       HTTP ハンドラ・セッション認証
  router/        ルーティング・共通 middleware
  domain/        ドメインモデル・初期プロンプト定義
  web/           管理画面フロントエンド（Vanilla JS / ES modules）
docker/          Dockerfile・段階的マイグレーション（docker/NN-*.sql）
deploy/          Kubernetes / GKE マニフェスト（Kustomize）と構築手順
docs/            設計ドキュメント
asset/
  dummy-sleep/   OCR 動作確認用の架空の睡眠アプリ画面
```

- `control` と `agent` は DB ロールも分離しており、`agent` は実データテーブルへの書き込み権限や Webhook URL の読み取り権限を持たない。
- tool の `user_id` は task の所有者を closure に束縛し、モデルの引数からは指定させない。

## 設計ドキュメント

| ドキュメント | 内容 |
|---|---|
| [`docs/concept.md`](docs/concept.md) | 中心に置く考え方、催促の判断ループと persona、実装状況 |
| [`docs/architecture.md`](docs/architecture.md) | 本番・開発構成、queue 設計、closure による user 分離、DB ロール権限 |
| [`docs/pvt.md`](docs/pvt.md) | PVT（反応時間テスト）の背景・測り方・指標 |
| [`docs/hackathon.md`](docs/hackathon.md) | ハッカソン応募要件の控え |
| [`deploy/README.md`](deploy/README.md) | Kubernetes / GKE へのデプロイ・データ移行手順 |

## ローカルでの起動（Docker Compose）

1. `.env.sample` を `.env` にコピーし、DB パスワード・`APP_PASSWORD`（12 文字以上）・`GEMINI_API_KEY`・GCS 設定・`GCP_CREDENTIALS_FILE`（ADC JSON のパス）を設定する。
2. コンテナを起動する：

```sh
docker compose up --build -d
```

3. `http://localhost:8080` を開き、`.env` の `APP_USERNAME` / `APP_PASSWORD` でログインする。
   OCR の動作確認には [`asset/dummy-sleep/`](asset/dummy-sleep/) の架空の睡眠画面を使用できる。

### テストと日常操作

```sh
./suyasuyactl.sh dev test      # go vet / go test と DB スキーマ検証（tests/schema.sql）
./suyasuyactl.sh dev logs      # control / agent のログ表示
```

既存 DB へのスキーマ更新は `schema.sql` ではなく `docker/NN-*.sql` を番号順に適用する（`./suyasuyactl.sh dev migrate docker/NN-*.sql`）。
本番 GKE の構築・運用手順は [`deploy/README.md`](deploy/README.md) を参照。
