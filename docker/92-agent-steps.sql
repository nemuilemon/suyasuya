-- agent の実行記録（job_steps）を追加する。既存 DB に管理者で適用する。control / agent は動かしたままでよい
-- （agent は表が無い間の記録失敗を log に出すだけで、task は止まらない）。
BEGIN;
-- agent の実行記録。1 回の claim（lease）が 1 run。model の各ラウンド・tool 呼び出し・結果を順に残す。
-- agent は lease を持っている間だけ追記できる。本人が「何を見て、どう判断したか」を読み返すためのもの。
CREATE TABLE IF NOT EXISTS job_steps (
    job_id uuid NOT NULL,
    user_id uuid NOT NULL,
    run_id uuid NOT NULL, -- claim 時の lease_token。再試行は別 run になる
    seq integer NOT NULL CHECK (seq >= 1),
    kind text NOT NULL CHECK (kind IN ('start', 'model', 'tool', 'final', 'error')),
    name text, -- model 名・tool 名・task 種別
    input jsonb, -- tool の引数、start では task の入力
    output jsonb, -- tool の結果・model の本文と要求した tool・最終結果。8KB を超えると preview に切り詰める
    ok boolean NOT NULL DEFAULT false,
    duration_ms bigint CHECK (duration_ms >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (job_id, run_id, seq),
    FOREIGN KEY (job_id, user_id) REFERENCES jobs(id, user_id)
);
CREATE INDEX IF NOT EXISTS job_steps_user ON job_steps(user_id, created_at DESC);
GRANT SELECT, INSERT, UPDATE, DELETE ON job_steps TO suyasuya_control;
GRANT INSERT ON job_steps TO suyasuya_agent;
COMMIT;
