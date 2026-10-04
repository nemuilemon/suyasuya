# suyasuya

**定時に寝ることを支える agent。**

第 5 回 Agentic AI Hackathon with Google Cloud 応募作品（開発中）。

## つくるもの

睡眠アプリの多くは、user に「つける」ことを求める。
だが**記録という行為そのものが、対象への注意になる**。
寝つきのように、意識するほど遠のく種類のものでは、それが逆に働くことがある。

だから suyasuya は **記録の手間を減らし、行動を支える。**

睡眠はデバイスが自動で測る。**screenshot を agent が読み取り、user が管理 UI で
確認・修正して承認する。画像なしで主要数値だけ手入力してもよい。**
夜の睡眠に加えて昼寝も記録でき、特に誤検知のある昼寝は追加・修正・除外できる。
承認した記録の蓄積・傾向の把握・声のかけ方の判断は、agent の側でやる。

そのうえで agent は **Discord で催促する。**「今から寝ろ」と。

介入先を睡眠時間ではなく**就寝時刻**に置いた。
長さは意志で増やしにくいが、時刻は寄せられる。
睡眠の規則性は、死亡リスクの予測力で睡眠時間を上回るという報告もある（Windred ら, 2024）。

| | |
|---|---|
| **催促** | 今夜送るか、どの強度で送るかを判断して Discord に投げる |
| **観測** | 翌日上がった記録から、実際に就寝時刻が寄ったかを読む |
| **学習** | 効いていない打ち方をやめ、効く打ち方に寄せる |
| **根拠** | PVT（反応時間）で、主観だけでは気づきにくい日中の機能の変化を捉える |

agent の人格は各ユーザーの base prompt で定義する。「ありす」は nemuilemon の agent であり、全員共通の人格ではない。

スコアを毎朝突きつけて習慣化を迫る形は取らない。
**agent が読んで、必要なときにだけ声をかける。**

## user に求めるもの

2 つだけ。

- **睡眠記録の確認・承認** —— screenshot の読み取り結果、または主要数値の手入力。昼寝も対象
- **PVT 1 分**（能動的な入力）—— 日中の機能の計測

PVT の測定時刻は user が設定する。

## 技術

- **Go** —— agent loop は framework を使わず自前で書く
- **Gemini** —— 画像の読み取り・agent の判断・文面生成のすべて
- **Docker Compose + Cloudflare Tunnel** —— 自分専用の local dev（Access で本人のみ許可）
- **Google Cloud（GKE）** —— 提出直前に移行する本番環境
- **PostgreSQL / Redis** —— 記録の真実と job queue
- **Cloud Storage** —— 画像保存。browser から署名付き URL へ直接 PUT
- **Discord** —— 催促の出口（`Notifier` で切ってあり、差し替え可能）

agent loop の判断手順・tool 選択・終了条件を含むプロンプト / skill は PostgreSQL に保存し、
control の管理 UI で編集・微調整できる設計にする。
Pixel Watch のデータ取り込み・睡眠記録との自動関連付けに向けた schema も用意する（連携は未実装）。

DB の初期定義は [`schema.sql`](schema.sql)。PostgreSQL 16 以上の空の DB に、
role を作成できる管理者で適用する。

```sh
psql -v ON_ERROR_STOP=1 -d suyasuya -f schema.sql
psql -v ON_ERROR_STOP=1 -d suyasuya -f tests/schema.sql
```

control / agent の role は `NOLOGIN` で作成する。`.env` のパスワードを使った
ログイン設定は今後の bootstrap で行う。アプリ・Compose は未実装。

## 設計

開発中。`docs/` に置いている。

| | |
|---|---|
| `concept.md` | 何を中心に置くか。催促の判断と persona |
| `pvt.md` | 反応時間テストの取り込み |
| `architecture.md` | 本番構成、closure による user 分離と DB role の操作権限 |
| `hackathon.md` | 応募要件 |

## データについて

`data/` 以下は開発用のサンプル。実際の睡眠記録を含むため、
公開範囲を変えるときは中身を確認すること。

## MVP の起動

`.env.sample` を `.env` にコピーし、DB の3つのパスワード、`APP_PASSWORD`（12文字以上）、
`GEMINI_API_KEY` と GCS 設定を入れる。初回起動で管理者を作成する。

```sh
docker compose up --build -d
```

http://localhost:8080 を開き、`.env` の `APP_USERNAME` / `APP_PASSWORD` でログインする。
control・agent・PostgreSQL・Redis を起動する。DB/Redis の port はホストへ公開しない。
既存 Cloudflare Tunnel の転送先は `http://localhost:8080`。
HTTPS 公開では `APP_SECURE_COOKIE=true` とし、公開 origin を bucket の CORS に追加する。

### Cloud Storage の認証

local では `gcloud auth application-default login` の ADC を使用できる。
`.env` の `GCP_CREDENTIALS_FILE` に ADC JSON の絶対パスを設定する。
Compose はこの1ファイルを read-only で mount する。秘密鍵の発行は不要。
実行ユーザーは `LOCAL_UID` / `LOCAL_GID`（既定 1000）とし、ADC の読み取り権限に合わせる。
ADC の identity は bucket の object 読み取り権限と、署名用 SA の
`roles/iam.serviceAccountTokenCreator` が必要。署名用 SA は bucket の作成・読み取り権限が必要。
`GCS_CORS_ALLOWED_ORIGINS` は構築用の控えであり、アプリが CORS を自動変更することはない。

### 利用できる機能

- 主睡眠・昼寝の手入力、訂正、削除、変更履歴
- browser から GCS へ署名付き PUT → OCR job → 確認・承認／却下
- Gemini による OCR、失敗時の再試行と中止、agent 停止後の期限付き job 復旧
- PVT 1分測定、生データ保存、反応時間の集計、中断記録
- 測定時刻・就寝時刻・timezone の設定、OCR / agent loop のプロンプト版管理

Discord に就寝・PVT の通知を送る。複雑な agent 判断・watch 自動取り込みは未実装。
OCR は job 作成時の指示の版に固定し、再試行でも同じ版を使う。
画像内の日付や主睡眠／昼寝が不明な場合は、ユーザーが確認して入力する。
PVT はブラウザーの timing を用いる簡易測定で、医療機器としての測定ではない。

### 運用と確認

```sh
go test ./...
docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < tests/schema.sql
docker compose logs --tail=50 control agent
```

`schema.sql` と DB role の password は空の DB の初回起動時だけ適用する。
既存 DB への変更は migration として適用する。`.env` の変更だけでは既存の
管理者パスワード・DB role のパスワードは変更されない。
DB と queue は named volume に永続化する。`docker compose down` で停止できる。
`down -v` は記録も削除するため、通常は使わない。
未完了 upload や削除した記録の GCS 原画像は自動削除しない。保持期限の掃除は今後対応する。

## コード構成

- `cmd/control/main.go`：control 用 DB role、service、HTTP server を組み立てる。
- `cmd/agent/main.go`：agent 用 DB role、Gemini OCR / 催促 service、queue の処理 loop を組み立てる。
- `internal/router`：HTTP route と共通 middleware。
- `internal/handler`：HTTP 入力・認証 cookie・response。SQL は持たない。
- `internal/service`：承認・記録更新・PVT 集計などの処理。HTTP request/response は持たない。
- `internal/service/ocr`：Gemini による画像の読み取り。
- `internal/repository`：DB 接続、SQL、scan、transaction。ユーザー向け query は user_id で絞る。
- `internal/agent`：Redis queue の取得、lease・中止・結果確定を共通管理。
- `internal/agent`：目的に基づく判断、tool 呼び出し、モデルとの対話ループ。OCR は専用 service を維持する。
- `internal/notifier`：Discord 配信と送信確認。
- `internal/domain`：入力・結果・エラーの型。
- `internal/web`：管理画面。`#/record` は入力・OCR 確認、`#/records` は保存済み一覧。

control と agent は別バイナリ・別イメージとしてビルドする。
agent は router・handler・管理画面を import しない。control は Gemini の SDK を import しない。

管理画面の JS は ES modules で役割ごとに分ける。`app.js` は初期化、`auth.js` は
セッション確認、`router.js` は画面遷移、`pages-*.js` は各画面、`record-editor.js` は
記録フォーム、`api.js` は通信を担当する。ビルド用の JS framework は不要。

タスクの削除は `jobs.deleted_at` による一覧からの削除。承認済みの睡眠記録と出典は保持し、
未承認の候補は以後承認できない。実行中のタスクは lease を失効させて中止する。
既存 DB には `docker/40-job-deletion.sql` を適用してから更新する。

agent はタスク種別と handler の対応を `cmd/agent/main.go` で登録する。
取得・lease・中止・結果保存は `internal/agent` が共通で扱い、handler は
`domain.Task`（所有者・種別・JSON input 等）から JSON の結果を返す。
現在の登録は `extract_sleep` と `plan_reminder`。未登録の種別は失敗として保存する。
タスク追加時は handler と投入元を追加する。画面の画像承認は OCR 専用。

設定画面で Discord Webhook URL を保存・変更・削除できる。
URL は control 専用の `notification_destinations` に保存し、API には設定有無だけを返す。
Webhook 保存だけでは送信しない。「自動通知」を有効にすると設定時刻に送信する。
「接続テストを送信」は自動通知の ON/OFF にかかわらず1回送信する（1分に1回まで）。

既存 DB には `docker/50-agent-tasks.sql` を適用する。
旧 worker を停止して agent に切り替える。旧 Redis queue に残った pending job は
PostgreSQL から新しい `tasks:pending` に再投入され、実行中 job は lease 期限後に復旧する。

## 自動通知

- control が5秒ごとにユーザーの timezone と設定時刻を確認。就寝・PVT を各1日1回、PostgreSQL の一意キーで重複防止する。
- `plan_reminder` は作成時の `agent.loop` / `job.plan_reminder` の版を固定。管理画面で文面の指示を調整できる。
- agent は目的とユーザーの base prompt に沿って tool を選び、送信の要否・本文・理由を返す。生成失敗時は定型文を送らず task の失敗として残す。
- 起動条件は設定時刻。agent は tool で承認済み睡眠記録・PVT・設定・通知履歴を参照できる。介入効果を学習する処理は未実装。
- control が結果を通知テーブルに移し、送信直前に ON/OFF・時刻・timezone・送信先を再確認する。agent は webhook URL を読めない。
- 再起動後は予定時刻から15分未満のみ追いつき送信。古い催促をまとめて送らない。停止した日の同じ通知は再有効化しても重ねて送らない。
- Discord の受領 ID を保存し、メンションを無効化する。429 は指定時間を待って最大3回まで送信を試みる。
- 通信切断・5xx・プロセス停止など結果が不明な送信は自動再送しない。履歴に失敗として残す。外部送信の exactly-once は保証しない。
- 設定画面で自動通知の停止・接続テスト・履歴確認ができる。送信中に停止した場合、その1件は取り消せない。

既存 DB には起動前に `docker/60-notification-delivery.sql` を適用する。
通知の DB 統合テストは `schema.sql` を適用した使い捨て DB を指定し、
`REMINDER_TEST_DB=<DB名>` と PostgreSQL 管理者の `POSTGRES_USER` / `POSTGRES_PASSWORD`
を渡して `go test ./internal/repository -run TestReminderDeliveryIntegration` を実行する
（DB は localhost:5432）。通常の `go test ./...` ではこの統合テストを skip する。

Discord の仕様：[Webhook execution](https://docs.discord.com/developers/resources/webhook#execute-webhook)、
[Rate limits](https://docs.discord.com/developers/topics/rate-limits)。

## 睡眠のふりかえり・週間レポート

「ふりかえり」で期間を指定し、睡眠時間（主睡眠・昼寝）と就寝時刻のチャートを確認できる。
集計対象は現在の承認済み記録。未記録や時間不明をゼロとして扱わず、平均主睡眠は
主睡眠の時間がすべて分かる日だけで計算する。同日の主睡眠が複数ある場合は時間を合算し、
最初の開始時刻を就寝時刻とする。日付は睡眠記録の日付（主睡眠は起床日）。
就寝時刻の表示・平均はユーザーの timezone で正午を日付境界として計算する。

週間レポートは `weekly_report` task。control が承認済み記録と集計の snapshot を入力に保存し、
agent が目的に沿って対象週の snapshot を tool で取得し、レポート内容を判断する。生成に失敗しても集計 snapshot は残る。
元の記録を編集・削除すると、未送信レポートの配信を止め、画面に更新が必要なことを表示する。

- 手動：対象週（月〜日）を選んで生成。既定では画面のみ。「Discord にも送る」を選ぶと送信する。
- 自動：設定の「自動通知」と「週1回、睡眠のまとめ」を有効にする。既定は月曜09:00、前週の月〜日を送る。
- 曜日・時刻・timezone は設定可能。同じ対象週は自動で1回だけ。停止後24時間未満なら追いつき送信する。
- 手動生成は途中の週にも対応。生成時点より先の日付の記録は含めない。
- 週次レポートの指示は設定の `job.weekly_report` で版管理する。

既存 DB には `docker/70-weekly-reports.sql` を適用する。

## Agent 共通の指示

設定の「Agent 共通の指示」（保存キー `agent.loop`）を全 agent task のベースとする。
`jobs` 作成時の DB trigger が有効な版を `job_config_versions` に固定し、agent の共通処理が
`Task.BasePrompt` に読み込む。OCR は `Task.Instructions(専用指示)` で画像抽出の指示と組み合わせる。
催促・週次レポートは共通指示を system に置き、タスクの目的・前提と利用可能な tool を渡す。
新しい handler も共通指示を引き継ぐ。
再試行は元の版を使う。共通指示が取得できないタスクは、指示なしでは実行せず失敗として残す。
既存 DB には control / agent を停止して `docker/80-task-base-prompt.sql` を適用する。
成功済みタスクの履歴は書き換えず、それ以外の未束縛タスクには移行時の共通指示を紐づける。

## 目的・tool・ユーザーごとの人格

`internal/agent` に `Reasoner`、tool registry、Gemini function-call loop を集約する。
OCR は従来の画像抽出のまま。催促と週間レポートには目的・前提・許可した tool 名を渡し、
どう催促するか、褒めるか、送らないかは agent が判断する。送信を見送る場合も理由を保存する。
外部送信・時刻スケジュール・HTTP は control、SQL は repository の責務とする。

- `getSleepTime`：本人の承認済み主睡眠・昼寝・日別集計。週次 task では入力 snapshot を使う。
- `getPVTResults`：本人の測定結果と protocol、完了状態。
- `getUserSettings`：表示名・timezone・目標時刻・通知設定・現地日付。
- `getNotificationHistory`：本人の直近20件の通知。相手からの返信を受信する機能ではない。

tool の user ID は task の所有者を closure に固定し、モデルには指定させない。
最大4回のtool対話・実tool呼び出し8回。最後に構造化した判断を生成する。
DB資格情報・webhook URL を tool に出さず、送信・書き込み tool は提供しない。
Gemini の function call ID と thought signature を保った履歴を返す。
参考：[Gemini function calling](https://ai.google.dev/gemini-api/docs/function-calling)。

プロンプトは `user_id` ごとに版管理し、一般ユーザーも自分の base / 目的を編集できる。
表示名も設定で変更でき、空欄では Discord Webhook 自体の名前を使う。共有コードに人格を固定しない。
`docker/90-agent-goals.sql` は未編集の既定プロンプトだけを目的中心の新版にする。
`docker/91-personal-agents.sql` は既存プロンプトを作成者の所有へ移行する。base 本文は保持する。

## agent ログ

管理画面の「agent ログ」で、task ごとに agent が何をしたかを読み返せる。直近50件の task（画像の読み取り・催促・週間レポート）を
一覧し、開くと次の順で表示する。

- **開始**：task の種別と入力
- **モデル**：各ラウンドの本文、要求した tool、token 数（入力・出力・思考）。最後のラウンドは最終回答（構造化 JSON）
- **tool**：tool 名・引数・結果・成否・所要時間。上限超過（`tool_call_limit_reached`）や引数エラーも失敗として残る
- **結果 / エラー**：handler が返した最終出力、または失敗理由

記録は `job_steps` に保存する。1 回の claim（lease）が 1 回の実行で、再試行は別の実行として並ぶ。
agent は lease を持っている間だけ追記でき（UPDATE / DELETE 権限なし）、中止・削除で lease を失った実行は書き込めない。
1 項目 8KB を超える引数・結果は preview に切り詰める。記録の書き込みに失敗しても task は止めず、log に残す。
thought signature は記録しない。tool の結果には本人の睡眠記録が含まれるため、読めるのは task の所有者だけ。

既存 DB には `docker/92-agent-steps.sql` を適用する（control / agent は動かしたままでよい）。
古い記録の自動削除はまだない。

## Kubernetes / GKE へのデプロイ

日常の操作は `./suyasuyactl.sh` にまとめてある（`dev up` / `dev test` / `gke deploy` / `gke migrate docker/NN-*.sql` など。引数なしで一覧）。
初回構築は [deploy/README.md](deploy/README.md) にマニフェスト・イメージ作成・認証・HTTPS 公開・Compose からのデータ移行手順をまとめている。
