# アーキテクチャ（本番）

**状態：方針決定。未実装。**

## 構成

本番は **GKE** に置く。container は 2 つに割り、DB は **PostgreSQL**、
control と agent の間は **Redis** を queue として挟む。

| container | 役割 |
|---|---|
| **control** | web app 本体。HTTP / UI、screenshot の受け口、通知の発火、job の投入。**schema と migration を所有する。** |
| **agent** | agent loop。queue から job を取り、Gemini を呼ぶ（画像からの読み取り、提案文の生成）。状態を持たない。 |

両者とも Postgres に接続するが、**agent は自分の role の権限内でだけ触る**。
実データ table は READ-ONLY、書けるのは結果系だけ。DDL は持たない。

境界を「どの container か」ではなく **DB の権限**で引く。
Go 側の配線ミスでは越えられない場所に線が置かれる。

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
screenshot upload
  → Postgres に行を作る（状態: pending）
  → Redis に job_id を積む
  → 即 202 を返す
```

結果は後から UI に流れてくる。upload の応答では待たない。

### 進捗と結果の戻り

**結果は agent が自分で Postgres に書く。** Redis を経由して control に返さない。

Redis に戻すと、上で置いた原則（`Redis が運ぶのは id だけ`）を
結果の経路だけが破ることになる。Gemini の出力は**金を払って得た実体**で、
list が飛べば再投入＝再課金でしか取り戻せない。
生成できた時点で真実を Postgres に着地させるのが筋。

control を経由しない副次的な効果として、**二重書きが構造的に起きない**。
その job を実行した worker は 1 つしかいないので、control を何台に増やしても関係ない。
結果を受けるためだけの list + `BLMOVE` は要らなくなる。

agent 側の書き方：

- 結果の INSERT と `jobs.status` の更新は**同一 transaction**
- `job_results(job_id)` に **unique 制約**を張り、`ON CONFLICT DO NOTHING`
  at-least-once で同じ job が 2 回走っても行が増えない
  （[取り方](#取り方) の冪等要求を、ここでは DB 制約として具体化する）

そのうえで **Redis に残る戻りは進捗の pub/sub 1 本だけ**になる。
完了通知も同じ channel でよい。

通知が揮発してよい理由は、真実がもう Postgres にあるから。
落ちても UI の更新が遅れるだけで、[未処理の行を拾い直す sweeper](#真実は-postgres-に置く) が回収する。
list で守っていた頃と違い、**通知の取りこぼしが仕事の消失にならない。**

### agent の role

権限は Postgres 側で絞る。Go の配線に依存させない。

```sql
-- agent 専用 role。DDL なし、DELETE なし
GRANT SELECT ON sleep_records, pvt_results          TO suyasuya_agent;
GRANT INSERT ON job_results                         TO suyasuya_agent;
GRANT UPDATE (status, finished_at) ON jobs          TO suyasuya_agent;
```

- **列単位で絞れる**。「結果 table 書き込みも可」を宣言として正確に書ける
- **migration は control（または独立 job）が所有する。** agent に DDL を渡さない
- **agent role を table の owner にしない。** owner は RLS を素通りする。
  owner に持たせる必要がある場合は `ALTER TABLE ... FORCE ROW LEVEL SECURITY`

agent は screenshot 経由の外部入力を LLM に食わせる、
**prompt injection を最も浴びる container**。だが tool は domain 固有で任意 SQL を通さず、
role は上記に限られ、さらに RLS が乗る。injection が通っても
「その user の行を読む」以上には届かない。

### UI への push

SSE でよい。WebSocket を持ち出す必要はない。
control が該当 job の進捗 channel を subscribe し、browser に流す。

**subscribe 時に所有者を検査すること。**
job_id を知っているだけで他人の進捗が読めてはいけない。
tool 層・DB 層で引いた境界を、ここでも引く。

### 認証

`hackathon.md` より、審査のため
**ソーシャルログインやメール受信が必須の方式は避ける**。
テストアカウントで審査員が入れる必要がある。

→ control の認証は **username / password の自前**に寄せる。
Google OAuth を入口にしない。

## ハッカソン要件との整合

`hackathon.md` の必須条件は、この構成でも**両方満たす**。

| 条件 | 充足 |
|---|---|
| 1. Google Cloud アプリケーション実行プロダクト | **GKE**（許可リストに含まれる） |
| 2. Google Cloud AI 技術 | **Gemini API** |

## agent の tool とデータ分離

**framework は使わない。tool は自前の Go で完結させる。**

### 原則：LLM に user_id を書かせない

queue が運ぶのは `job_id` / `user_id` といった**制御情報だけ**。
これを agent の実行文脈に**注入**し、tool call の query には
**注入された値が自動で入る**。LLM は user_id に触れない。

これがこの設計の要点になる。
一般的な tool calling では、モデルが `get_sleep_records(user_id: "...")` のように
**識別子を自分で書く**。するとデータ分離の保証が、
「モデルが正しい id を書くか」という**確率的な性質**にぶら下がってしまう。
prompt injection が刺されば他人のデータに手が届く。

ここでは user_id を **tool の引数から消す**。
モデルが書けない以上、間違えようがない。
分離が**構造で保証される**。

### 実装：closure で束ねる

job ごとに tool set を組み立て、その時点の user_id を closure に閉じ込める。

```go
func buildTools(db *sql.DB, uid UserID) []Tool {
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

守るべき点：

- **schema に user_id を載せない。** 「来ても無視する」ではなく、
  Gemini に渡す宣言から**消す**。載っていれば、いつか誰かが配線してしまう。
- **tool set を job をまたいで使い回さない。** 常駐 process なので、
  cache や global に置くと A の束縛が B の job に残る。**唯一の事故経路がここ。**
  job ごとに `buildTools` を呼ぶ。
- **`type UserID string`** のような named type にする。
  素の `string` だと引数の取り違えが compile を通ってしまう。

### 二重化

closure だけに頼らず、DB 側にも同じ境界を引く。
Postgres の **RLS（Row Level Security）** を張り、接続時に
`SET LOCAL app.user_id` を渡して、行がそもそも見えないようにする。

closure の配線ミスと RLS の設定ミスが**同時に起きない限り**漏れない。
in-cluster Postgres でも RLS は普通に使える。

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
- **engine 名は `internal/db/postgres/` の外に出さない。**
  型名・file 名・package 名のどこにも出さない。出たら設計が漏れている。
- **接続は path ではなく設定で受ける。** host / port / db / credential。
- **migration は engine 中立な道具で。** `PRAGMA user_version` 方式は採らない。

### UserID との接続

repository の各 method は **`UserID` を第一引数に取る**。

これで「user に紐づかない query が存在しない」ことが、
**interface 定義を読むだけで確かめられる**ようになる。
agent の closure 束縛（上記）と同じ境界を、DB 側でも型で表現することになる。

RLS は Postgres 固有なので、**実装側に閉じる**。
interface が表明するのは「この user の範囲で読む」という意図だけ。
engine が変われば WHERE 句で実現してもよい。

### やりすぎない

目的は「明日 MySQL に載せ替えられること」ではない。
**engine の前提が domain に染み出さないこと**が目的。

実装は Postgres 1 つでよい。使わない実装を 2 つ書くのは無駄。
seam さえ通っていれば、必要になった時に足せる。

## agent loop

常駐。`BLMOVE` で queue を待ち、job を取って処理する。

- worker 数は上限を決める（Gemini の rate limit と費用に直結する）
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
一箇所 cancel すれば全部が巻き戻る。これは ctx が設計上まさに担う仕事。

`context.WithTimeout` も併せて張る。
tool call の反復上限（上記）と二重に、暴走を時間でも止める。

**ただし ctx で止まるのは process 内だけ。**
user の「中止」は control から来るので、container をまたぐ経路が要る。

```
control ──▶ Redis pub/sub (cancel channel) ──▶ agent
                                                 │
                                    job_id → cancel() の map を引く
```

agent は job ごとの `cancel` を map に持ち、cancel 通知で該当 job を落とす。
SIGTERM 時は root ctx を cancel して、全 job を一括で畳む。

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

同じ理由で、**user_id も ctx に入れない。**
job に固有の束縛は closure、process の生死は ctx。この線を混ぜない。

## 未決

### Redis の置き場所

| 案 | 利点 | 難点 |
|---|---|---|
| **in-cluster** | 安い。Deployment 1 つで済む | 落ちたら queue が飛ぶ（上の設計で許容済み） |
| **Memorystore** | 運用しなくていい | 最小構成でもクーポンを食う |

queue の中身を捨てても復旧できる設計にしたので、**in-cluster で足りる**。
要決定だが、締切を踏まえると in-cluster 推し。

### DB の置き場所（暫定：in-cluster）

一旦 **in-cluster** で進める。StatefulSet + PVC。

注意点を 1 つだけ記録しておく。
cluster を作り直すと**データが消えうる**。審査用のテストアカウントと
サンプルデータは、**再投入できる seed script を用意しておくこと**。
PVT の実測データ（`pvt.md` 参照）は積み上げに lead time があるので、
消すと取り返しがつかない。**定期的に dump を手元に落とす。**

不安が残るなら Cloud SQL に移すが、その判断は後でよい。

## 運用上の注意

**着手前に一度だけ判断し直すこと。**

- **GKE は cluster の管理費がかかる。** クーポン($300) を食う速度に直結するので、
  Autopilot か、無料枠の対象になる構成かを確認する
- **Cloud Run に比べて ops が明確に重い。** 残り日数に対して構築コストが見合うか
