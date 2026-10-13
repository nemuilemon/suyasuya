# アーキテクチャ（開発・本番）

**状態：実装済み。2026-10-10 から GKE Autopilot（asia-northeast1）で稼働している。**
設計段階から変えた点は、末尾の「設計からの変更」にまとめた。

## 構成

**本番は GKE Autopilot、開発は local の Docker Compose。**

- **本番**：control・agent・PostgreSQL・Redis を namespace `suyasuya` に置く。
  PostgreSQL と Redis は StatefulSet + PVC。公開は cluster 内の cloudflared（Cloudflare Tunnel、
  2 replica + PDB）が control の Service に繋ぐ。外部 LB と Ingress は持たない。
  NetworkPolicy で、control に入れるのは cloudflared と agent だけにしている。
  Pod ごとに Workload Identity で別の IAM service account を使い、GCS の権限を control と agent で分ける。
  手順は [`deploy/README.md`](../deploy/README.md)。
- **開発**：Compose。Cloudflare Tunnel 経由で使い、Cloudflare Access で本人だけを許可する。
  Tunnel は **ホストの既存 daemon を流用**する。Compose に cloudflared は追加せず、
  suyasuya の環境変数に Tunnel token は持たない。control の port はホストの loopback に公開する。
  local の停止中は通知・task 処理も止まる。

アプリの container は 2 つに割り、DB は **PostgreSQL**、
control と agent の間は **Redis** を queue として挟む。
画像の読み取り・agent の判断・文面生成のモデルは **すべて Gemini** を使う。
専用 API key は local では Git 管理外の `.env`、本番では Secret から渡す。

| container | 役割 |
|---|---|
| **control** | web app 本体。HTTP / UI、画像 PUT 用の署名付き URL 発行と完了確認、通知の発火、job の投入。**schema と migration を所有する。** |
| **agent** | agent loop。queue から task を取り、Gemini を呼ぶ（画像からの読み取り、催促の判断と文面、週間レポート）。状態を持たない。 |

両者とも Postgres に接続するが、**agent は自分の role の権限内でだけ触る**。
実データ table は READ-ONLY、書けるのは結果系だけ。DDL は持たない。

**読み書きできる table・列は DB role の権限で制限する。**
user 間のデータ分離は、job ごとの closure に束縛した user_id と、
repository の所有者条件で実現する。**今回は RLS を使わない。**

```
           ┌──────────┐
 user ───▶│ control  │◀──────▶ PostgreSQL
           └────┬─────┘             ▲
                │                   │  role: 実データ READ-ONLY
             ┌──▼───┐               │        結果 table のみ WRITE
             │ Redis│  queue        │
             └──┬───┘               │
                │                   │
           ┌────▼─────┐             │
           │  agent   │─────────────┘
           └────┬─────┘
                └────▶ Gemini API
```

## queue

**Redis** を使う。control が job を積み、agent が取る。

### 真実は Postgres に置く

Redis が運ぶのは **job の id だけ**にする。実体は Postgres に書いてある状態を指す。

こうしておくと、**queue が飛んでも復旧できる**。
control が「未処理のまま一定時間経った行」を Postgres から拾って積み直せばいい。
Redis の永続化（AOF / RDB）をどう設定するかが、設計の急所でなくなる。

逆に job の中身を Redis に持たせると、Redis の耐久性がそのまま
データ損失に直結する。この規模でそこに神経を使う理由はない。

### 取り方

素朴な `LPUSH` / `BRPOP` は、agent が job を掴んだまま死ぬと消える。
`BLMOVE` で processing list に移し、完了時に消す形にする。
取りこぼしは上の再投入で回収できるので、**at-least-once** で十分。

→ agent 側の処理は**冪等**に書くこと。同じ job が 2 回来てもよいように。

## control

control は orchestrator であると同時に、**user が触る UI そのもの**。
web app として画面を返すのも、job を積むのも、同じ Go process が担う。

### 速さの異なる 2 つの仕事を同居させる

UI の HTTP は ms で返らないといけない。
agent の仕事は Gemini を挟むので秒〜分かかる。
**UI request を agent の完了で待たせない。**

```
browser → control：画像の upload を要求
  → control が uploads 行を作り、GCS の PUT 用署名付き URL を返す
browser → Cloud Storage：画像を直接 PUT
browser → control：upload_id の完了を通知
  → control が GCS の実体を確認し、uploads の確定と jobs の作成を行う
  → Redis に job_id を積み、202 を返す
```

結果は後から UI に流れてくる。upload の応答では待たない。

### 画像の保存先：Cloud Storage

**local dev から非公開の GCS bucket を使う。** 画像本体は browser から直接 PUT し、
PostgreSQL の `uploads` には bucket・object 名・サイズ・generation・状態だけを保存する。

- control が所有者に紐づく一意の object 名を決め、短い期限の **V4 signed URL** を発行する。
  HTTP method は PUT、Content-Type と新規作成条件（`x-goog-if-generation-match: 0`）を署名に含める。
  browser は指定されたヘッダーで送信し、同じ URL による上書きを防ぐ。
- bucket の CORS に dev / 本番の origin と PUT を設定する。画像の確認表示も署名付き GET を使う。
  CORS は本人認証の代わりにせず、URL の発行・完了 API で所有者を確認する。
- 完了 API は browser の申告だけで確定せず、GCS のサイズ・Content-Type を検証する。
  アプリのサイズ上限・申告値と合わない画像は却下する。agent も画像として読み込めることを検証する。
- 完了の再送で job が増えないよう upload_id から投入キーを作る。
  agent は保存済み generation の画像を IAM 認証で取得して Gemini に渡す。
  URL の有効期限に job の再試行を依存させない。
- 期限切れの未完了 upload は、GCS の残存 object と併せて定期的に掃除する。
  GKE 移行時も画像は GCS に残し、DB の参照を引き継ぐ。

署名用の service account と GCS 認証は Gemini API key とは別に用意する。
参考：[Signed URLs](https://cloud.google.com/storage/docs/access-control/signed-urls)、
[CORS](https://cloud.google.com/storage/docs/cross-origin)。

### 睡眠記録の確認・承認と手入力

**画像の読み取り完了と、睡眠記録の確定を分ける。** agent は読み取り候補を
`job_results` に保存して job を完了する。候補は未承認であり、
`sleep_records` への反映や催促の評価はまだ行わない。

```
画像 → agent の読み取り候補 ─┐
                            ├→ 管理 UI で user が確認・修正・承認
画像なし → 主要数値の手入力 ─┘   → control が sleep_records に保存
                                 → 評価 task（未実装）→ 応答・次の催促判断
```

- **本人の記録管理**：通常 user が自分の候補を確認し、修正・承認・却下できる。
  プロンプトの編集も本人の版だけに限る。他人の記録・版には触れない。
- **主要項目**：主睡眠 / 昼寝の種別、開始・終了日時、睡眠時間。
  日付とタイムゾーンを持ち、日跨ぎを扱う。画像や睡眠段階の詳細は必須にしない。
  不明な項目は欠測として保持し、時刻不明の記録を就寝時刻の評価に使わない。
- **昼寝**：同日に複数件を保存できる。検出漏れは手入力で追加し、誤検知は却下する。
  主睡眠との分類違い、時刻・睡眠時間も承認前に修正できる。
  記録がないことを自動的に「昼寝なし」と解釈しない。
- **確定経路**：control が所有者と入力値を検証し、承認した版・承認日時・
  入力元（画像 / 手入力）を保存する。画像由来では元の候補と user の修正も辿れるようにする。
  agent の実データへの書き込み権限は増やさない。
- **一度だけ反映**：承認対象の版または手入力の送信 ID で重複保存を防ぐ。
  記録の確定と評価 job の作成は同一 transaction とし、queue への再投入に対応する。
  承認後の訂正・削除も版として残し、古い評価を無効化して最新の確定状態で再評価する。
- **評価対象**：傾向分析・催促には有効な承認済み記録だけを渡す。
  昼寝は主睡眠と区別して集計し、昼寝単独の追加では昨夜の催促を再評価しない。

### Pixel Watch のデータ連携（schema の準備）

Pixel Watch 連携に向け、次の table も用意する（取得 API・同期処理は未実装）。

- `watch_devices`：端末・取得元・同期の有効 / 無効・取得位置。
- `watch_sleep_observations`：外部レコード ID と版を持つ取得原本。再取得を重複登録せず、元データの訂正も残す。
- `sleep_watch_links`：画像・手入力の記録と、時刻の重なりで自動照合した候補。user が確認・却下できる。

同期・関連付けの保存は control が担い、agent の実データへの書き込み権限は増やさない。
watch 由来の記録も本人の確認・承認後に `sleep_records` に反映する。
既に画像や手入力で登録済みなら関連付けを優先し、同じ睡眠を二重に集計しない。
特に昼寝の誤検知は自動で確定せず、原本と修正値を分けて保持する。
取得原本だけを承認済みの睡眠として分析しない。

### job の結果保存

**読み取り候補や評価結果は agent が自分で Postgres に書く。**
Redis を経由して control に返さない。承認済み睡眠記録への反映は上記の control が担う。

Redis に戻すと、上で置いた原則（`Redis が運ぶのは id だけ`）を
結果の経路だけが破ることになる。Gemini の出力は**金を払って得た実体**で、
list が飛べば再投入＝再課金でしか取り戻せない。
生成できた時点で真実を Postgres に着地させるのが筋。

結果を受けるためだけの list + `BLMOVE` は要らない。
ただし同じ job が複数回実行されうるため、結果の重複は DB 制約で防ぐ。

agent 側の書き方：

- 結果の INSERT と `jobs.status` の更新は**同一 transaction**
- `job_results(job_id)` に **unique 制約**を張り、`ON CONFLICT DO NOTHING`
  at-least-once で同じ job が 2 回走っても行が増えない
  （[取り方](#取り方) の冪等要求を、ここでは DB 制約として具体化する）

そのうえで **Redis に戻す経路は持たない。** 進捗の pub/sub も置かなかった。
UI は画面を開いている間、Postgres の状態を数秒おきに照会する（下の「UI への反映」）。
sweeper は未処理 task の再投入を担い、完了済み task の表示更新とは分ける。
真実が Postgres にあるので、**表示の取りこぼしが仕事の消失にならない。**

### agent の role

table・列への操作権限は Postgres 側で絞る。user ごとの範囲はアプリ側で制限する。
実際の table・制約・GRANT は [`schema.sql`](../schema.sql) を正本とする。
以下は権限の概要。agent の claim / 再試行用の列も schema 側で定義する。

```sql
-- agent 専用 role。DDL なし、DELETE なし
GRANT SELECT ON sleep_records, pvt_sessions         TO suyasuya_agent;
GRANT SELECT ON jobs                               TO suyasuya_agent;
GRANT SELECT ON agent_config_versions              TO suyasuya_agent;
GRANT INSERT ON job_results                         TO suyasuya_agent;
GRANT UPDATE (status, finished_at) ON jobs          TO suyasuya_agent;
```

- **列単位で絞れる**。「結果 table 書き込みも可」を宣言として正確に書ける
- **migration は control（または独立 job）が所有する。** agent に DDL を渡さない
- **agent role を table の owner にしない。** migration 用の権限を渡さず、
  実行に必要な操作だけを GRANT する。

agent は screenshot 経由の外部入力を LLM に食わせる、
**prompt injection を最も浴びる container**。だが tool は domain 固有で任意 SQL を通さず、
role は上記に限る。tool が使う user_id は closure に固定し、
repository はその user の行だけを扱う。LLM の引数で他人の user_id を指定できない。

### UI への反映

**polling にした。** SSE も WebSocket も使っていない。
記録画面は 5 秒おきに task 一覧を、ふりかえり画面は 3 秒おきにレポート一覧を、その画面を開いている間だけ取り直す。
SSE はこの規模には過剰（over-spec）なので入れていない。

所有者の検査は通常の API と同じ。認証済み user の task しか一覧に出ない。
task_id を知っているだけで他人の進捗が読めることはない。

### 認証

`hackathon.md` より、審査のため
**ソーシャルログインやメール受信が必須の方式は避ける**。
テストアカウントで審査員が入れる必要がある。

→ control の認証は **username / password の自前**に寄せる。
Google OAuth を入口にしない。

### user の測定時刻設定

**PVT の測定時刻は user が設定する。** control の設定 UI で、本人の
測定時刻とタイムゾーンを PostgreSQL に保存する。
control の scheduler はこの設定に従って測定通知を発火する。
実際の測定日時は PVT session に別途記録する（`pvt.md`）。

### agent のプロンプト / skill 管理

**agent 用のプロンプトと skill は PostgreSQL を正本にする。**
control に管理 UI を置き、再デプロイせずに内容を微調整できるようにする。
ここでいう skill は、agent に渡す作業手順・判断指針の定義。

**agent loop の制御プロンプトも管理 UI の編集対象にする。**
どの情報を先に見るか、どの tool を呼ぶか、結果を受けて追加調査するか、
材料不足で保留するか、いつ回答を確定して終了するかを微調整できる。

| config_key | 調整する内容 |
|---|---|
| `agent.loop` | 全 task 共通の指示（判断の根拠、材料不足なら保留、不要な tool を呼ばない、など） |
| `job.extract_sleep` | OCR の読み取り指示、欠測・曖昧な値の扱い |
| `job.plan_reminder` | 催促の目的（就寝・PVT を支える。必要なデータを tool で確かめ、送るかと内容を決める） |
| `job.weekly_report` | 週間レポートの目的 |

承認済み記録の評価（`job.evaluate_sleep`）と、persona を別の key に分けることは未実装。
persona は各 user の `agent.loop` に書く。

共通の loop 指示と task 別の指示を組み合わせて Gemini に渡す。
OCR のように原則 1 回の呼び出しで済む task に、不要な tool loop は強制しない。
反復回数（4 ラウンド・tool 8 回）・timeout・tool の許可範囲・user 分離・記録の承認は
コード側で強制し、プロンプトの編集では解除できない。

- **保存先**：`agent_config_versions` に定義の識別子・本文・版・所有者・作成日時を保存する。
- **管理 UI**：設定画面で、各 user が自分の版を編集し、新しい版を有効化できる。過去の版は残る。
- **読み込み**：task 作成時に有効な版を `job_config_versions` に固定し、その task 内では変えない。
  再試行も同じ版を使う。判断履歴から、どの版で動いたかを辿れる。
- **権限**：agent は読み取り専用とし、プロンプトを自分で書き換える tool は持たない。
  tool の実装・利用可能な権限・実行上限はコード側で管理する。
- **初期値**：`internal/domain` の既定文を、user を作るときにその user の版 1 として入れる。
  既にある版は上書きしない。

## ハッカソン要件との整合

`hackathon.md` の必須条件は、この構成でも**両方満たす**。

| 条件 | 充足 |
|---|---|
| 1. Google Cloud アプリケーション実行プロダクト | **GKE**（許可リストに含まれる） |
| 2. Google Cloud AI 技術 | **Gemini API** |

## agent の tool とデータ分離

**framework は使わない。tool は自前の Go で完結させる。**

### 原則：LLM に user_id を書かせない

queue が運ぶのは **job_id だけ**。
agent は Postgres の job から所有者の user_id を取得し、closure に束縛する。
control は job 作成時に認証済み user の ID を保存する。
tool call の query にはこの値が自動で入り、LLM は user_id に触れない。

これがこの設計の要点になる。
一般的な tool calling では、モデルが `get_sleep_records(user_id: "...")` のように
**識別子を自分で書く**。するとデータ分離の保証が、
「モデルが正しい id を書くか」という**確率的な性質**にぶら下がってしまう。
prompt injection が刺されば他人のデータに手が届く。

ここでは user_id を **tool の引数から消す**。
これにより、モデルが読み取り先の user を選ぶ経路をなくす。
分離は、正しい所有者の束縛と repository の user 条件によって成立する。

### 実装：closure で束ねる

job ごとに tool set を組み立て、その時点の user_id を closure に閉じ込める。

```go
func buildTools(db DB, uid UserID) []Tool {
    return []Tool{
        {
            Name: "get_sleep_records",
            // schema に user_id は存在しない
            Schema: `{"days": {"type": "integer"}}`,
            Call: func(args Args) (any, error) {
                return querySleep(db, uid, args.Days) // uid は closure から
            },
        },
        // ...
    }
}
```

実装は `internal/agent/tools.go` の `NewToolRegistry(data, task)`。task の所有者 ID は
claim 時に DB から読む。tool の宣言（`getSleepTime`・`getPVTResults`・`getUserSettings`・
`getNotificationHistory`）に user_id は無い。

守るべき点：

- **schema に user_id を載せない。** 「来ても無視する」ではなく、
  Gemini に渡す宣言から**消す**。載っていれば、いつか誰かが配線してしまう。
- **tool set を job をまたいで使い回さない。** 常駐 process なので、
  cache や global に置くと A の束縛が B の job に残る。
  job ごとに `buildTools` を呼ぶ。
- **`type UserID string`** のような named type にする（未対応。いまは素の `string`）。
  素の `string` だと引数の取り違えが compile を通ってしまう。

### 今回の分離方式

**RLS は採用しない。** user_id を closure に束縛し、repository に明示的に渡す。
user 所有のデータを読む・更新する query は必ず `WHERE user_id = ...` で範囲を絞る。
record_id や job_id を指定する場合も、user 向け操作では所有者条件を併用する。
INSERT の user_id も束縛した値を使い、LLM や request body の指定を採用しない。

control の記録承認・手入力・設定変更も、認証済み user の範囲で実行する。
agent の job 取得、scheduler、管理者用設定は内部用の操作として分け、
LLM の tool や通常 user の API に全 user 向けの操作を公開しない。

## DB は interface で切る

### ANP-neo の教訓

ANP-neo は境界をちゃんと切った project だった。`llm.Client`、`rag.Retriever`、
`storage.ImageStore`、`ChatService`、`UserService` —— どれも interface が立っている。

**DB だけが切られなかった。** 結果として起きたことを記録しておく。

| 症状 | 実際 |
|---|---|
| service が `*sql.DB` を直接受け取る | `NewSQLiteChatService(d *sql.DB, ...)` |
| **engine 名が service 層まで漏れた** | `sqlite_chat.go` / `SQLiteChatService` |
| 接続の抽象が file 前提 | `db.Open(path string)` —— Postgres は path では開かない |
| migration 機構そのものが engine 固有 | `PRAGMA user_version` で版管理。Postgres に対応物がない |

いちばん効くのは 2 行目。**engine 名が service 層の型名に現れた時点で負けている。**
storage 層の都合を知らないはずの層が、知ってしまっている。
命名は、結合が漏れたことを知らせる警報として読める。

つまり suyasuya に必要なのは新しい規律ではない。
**他の境界で既にやっていることを、DB にも適用するだけ。**

### 規律

- **interface は domain の言葉で話す。** `SaveSleepRecord(ctx, uid, rec)` であって、
  `Query(sql string, args ...any)` ではない。
  `Query`/`Exec` を包んだだけの interface は**何も買っていない**。
  方言はそのまま通り抜け、呼び出し側は SQL を書き続ける。
- **engine 名は `internal/repository` の外に出さない。** SQL と driver はこの package に閉じた。
  型名・file 名・package 名のどこにも出さない。出たら設計が漏れている。
- **接続は path ではなく設定で受ける。** host / port / db / credential。
- **migration は番号付きの SQL ファイル（`docker/NN-*.sql`）を順に適用する。** `PRAGMA user_version` 方式は採らない。

### UserID との接続

user 所有のデータを扱う repository method は **`UserID` を明示的に取る**
（`ctx` がある場合はその次）。agent の closure から渡された ID を、
実装側で query の所有者条件に使う。

interface は「この user の範囲で扱う」という契約を表し、
内部用・管理者用の操作とは分ける。

### やりすぎない

目的は「明日 MySQL に載せ替えられること」ではない。
**engine の前提が domain に染み出さないこと**が目的。

実装は Postgres 1 つでよい。使わない実装を 2 つ書くのは無駄。
seam さえ通っていれば、必要になった時に足せる。

## agent loop

常駐。`BLMOVE` で queue を待ち、job を取って処理する。

- agent 数は上限を決める（Gemini の rate limit と費用に直結する）
- tool call のループには**反復上限**を置く。
  無限に tool を呼び続ける事故が、そのまま課金になる
- **SIGTERM で graceful shutdown。** 新規受付を止め、in-flight を流し切る。
  k8s の `terminationGracePeriodSeconds` を処理時間より長く取る
- 流し切れずに殺されても、at-least-once + 冪等 なので再投入で回収できる

goroutine と channel で素直に書ける範囲。framework を挟む理由はない。

### context の使い分け

**原則：ctx は「止める」ために使う。「運ぶ」ためには使わない。**

| 用途 | 手段 |
|---|---|
| 中止・timeout・shutdown | **ctx** |
| job 固有の束縛（user_id、進捗の報告先） | **closure** |

#### 中止：ctx が正解

job ごとに `context.WithCancel` を張り、全ての I/O に通す。
`QueryContext`、Gemini への `http.NewRequestWithContext`、tool call のループ。
cancel は進行中の処理に中止を伝える。保存済みの結果や送信済みの通知は巻き戻らない。

`context.WithTimeout` も併せて張る。
tool call の反復上限（上記）と二重に、暴走を時間でも止める。

**ただし ctx で止まるのは process 内だけ。**
user の「中止」は control から来るので、container をまたぐ経路が要る。

```
control ──▶ Postgres に中止要求（cancel_requested_at）を保存
agent   ──▶ 実行中は 1 秒おきに中止要求を確認 ──▶ 見つけたら、その task の cancel()
```

Redis の pub/sub は使わず、DB の確認だけにした（`internal/agent/process.go`）。
agent は開始前・実行中・結果確定時に中止要求を見る。
中止済み job は再投入しない。完了済み job への中止は完了を取り消さない。
SIGTERM 時は新規受付を止め、猶予時間内は実行中の job を待つ。
猶予を超えたら root ctx を cancel する。終了に伴う中断は user の中止と区別し、再投入できる。

#### 落とし穴：後始末に死んだ ctx を使わない

cancel したあと「job を中止済みにする」DB 書き込みを、
**同じ ctx で撃つと失敗する**。ctx はもう死んでいる。

`context.WithoutCancel` で切り離すか、新しい ctx を起こす。
これを踏むと「中止したのに状態が中断のまま残る」job が湧く。

#### 進捗：ctx に載せない

進捗を `context.Value` で運びたくなるが、これは避ける。

- ctx は**immutable**。値を更新できないので、
  結局 channel や pointer を中に隠して持ち回ることになる。これは密輸。
- `any` なので**型が消える**。取り違えが compile を通る。
- 関数 signature が「進捗を報告する」と**言わなくなる**。依存が隠れる。

progress reporter は **closure で束ねる**。user_id と同じ扱いにする。

```go
func buildTools(db DB, uid UserID, report ProgressFunc) []Tool
```

（進捗の報告は実装していない。UI は polling で状態を読む）

同じ理由で、**user_id も ctx に入れない。**
job に固有の束縛は closure、process の生死は ctx。この線を混ぜない。

## 決めたこと（旧「未決」）

### 本番 Redis の置き場所 → in-cluster

queue の中身を捨てても復旧できる設計にしたので、**in-cluster の StatefulSet** にした。
Memorystore は使っていない（最小構成でもクーポンを食うため）。

### DB の置き場所 → in-cluster

**StatefulSet + PVC**（`standard-rwo`）。Cloud SQL は使っていない。
移行時は local DB をバックアップし、記録と設定を本番へ引き継いだ。
dev の scheduler を止めてから本番を有効にし、通知の二重送信を防いだ。

cluster を作り直すと**データが消えうる**。PVT の実測データ（`pvt.md` 参照）は
積み上げに lead time があるので、消すと取り返しがつかない。**定期的に dump を手元に落とす。**
審査用のテストアカウントは、本人の記録を複製して作る script で作り直せる（script は Git 管理外）。

## 運用上の注意

- **GKE は cluster の管理費がかかる。** クーポン($300) を食う速度に直結するので、**Autopilot** にした
- 外部 LB の転送ルール 2 本（約 $18/月）を避けるため、公開は Ingress ではなく
  cluster 内の cloudflared にした（2026-10-10）
- Cloud Run に比べて ops は重い。日常の操作は `./suyasuyactl.sh`（`gke deploy` / `gke migrate` など）にまとめた

## 設計からの変更

| 設計 | 実装 |
|---|---|
| 提出直前に GKE へ移行 | 2026-10-10 から GKE Autopilot で稼働 |
| UI への push は SSE、進捗は Redis pub/sub | polling（画面を開いている間、3〜5 秒おき） |
| 中止は Redis pub/sub で agent に届ける | DB の中止要求を agent が 1 秒おきに確認 |
| 公開は GCE Ingress | cluster 内の cloudflared（Cloudflare Tunnel） |
| `internal/db/postgres/` | `internal/repository` |
| `type UserID string` | 未対応（素の `string`。所有者の束縛は closure と repository の条件で守る） |
| 承認後の評価 job、`persona.arisu` | 未実装。persona は各 user の `agent.loop` に書く |
