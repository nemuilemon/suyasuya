-- PostgreSQL 16+ / 空の suyasuya DB に管理者で一度だけ適用する。
-- psql -v ON_ERROR_STOP=1 -d suyasuya -f schema.sql
-- role の LOGIN / PASSWORD は bootstrap 側で .env から設定する。
-- RLS は使わない。user 向け query は必ず束縛した user_id で絞る。
-- updated_at は更新するアプリ側で設定する。
BEGIN;

DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'suyasuya_control') THEN
        CREATE ROLE suyasuya_control NOLOGIN;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'suyasuya_agent') THEN
        CREATE ROLE suyasuya_agent NOLOGIN;
    END IF;
END
$$;

REVOKE CREATE ON SCHEMA public FROM PUBLIC;

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username text NOT NULL UNIQUE CHECK (btrim(username) <> ''),
    password_hash text NOT NULL CHECK (password_hash <> ''),
    is_admin boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app_sessions (
    token_hash text PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id),
    expires_at timestamptz NOT NULL
);
CREATE INDEX app_sessions_expiry ON app_sessions(expires_at);

CREATE TABLE user_settings (
    user_id uuid PRIMARY KEY REFERENCES users(id),
    timezone text NOT NULL DEFAULT 'Asia/Tokyo', -- IANA 名はアプリで検証する
    target_bedtime time,
    pvt_reminder_time time,
    notifications_enabled boolean NOT NULL DEFAULT false,
    snoozed_until timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- webhook URL は資格情報を含む。agent に読ませない。
CREATE TABLE notification_destinations (
    user_id uuid PRIMARY KEY REFERENCES users(id),
    discord_webhook_url text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- 画像本体は非公開の Cloud Storage。browser が V4 signed URL で直接 PUT する。
-- URL 自体は保存せず、control が確定した object の generation を保存する。
CREATE TABLE uploads (
    capture_date date,
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    content_type text NOT NULL CHECK (content_type IN ('image/png', 'image/jpeg', 'image/webp')),
    bucket text NOT NULL CHECK (btrim(bucket) <> ''),
    object_name text NOT NULL CHECK (btrim(object_name) <> ''),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'uploaded', 'rejected')),
    expected_size_bytes bigint NOT NULL CHECK (expected_size_bytes > 0),
    size_bytes bigint CHECK (size_bytes > 0),
    generation bigint CHECK (generation > 0),
    expires_at timestamptz NOT NULL,
    uploaded_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, user_id),
    UNIQUE (bucket, object_name),
    CHECK (expires_at > created_at),
    CHECK (status <> 'uploaded' OR
        (generation IS NOT NULL AND size_bytes IS NOT NULL AND uploaded_at IS NOT NULL
         AND size_bytes = expected_size_bytes))
);
CREATE INDEX uploads_pending_expiry ON uploads(expires_at) WHERE status = 'pending';
-- control が所有者・GCS の実体を検証し、uploaded への更新と job 作成を同一 transaction で行う。

-- Pixel Watch を最初の対象にする。取得 API に依存しない端末・取得元の定義。
-- OAuth token などの資格情報はここに保存しない。
CREATE TABLE watch_devices (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    provider text NOT NULL CHECK (btrim(provider) <> ''), -- 取得経路の識別子。連携実装時に決める
    external_device_id text,
    model text NOT NULL DEFAULT 'Pixel Watch',
    sync_enabled boolean NOT NULL DEFAULT false,
    sync_cursor text,
    last_synced_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, user_id),
    UNIQUE (user_id, provider, external_device_id)
);

-- 取得した睡眠区間の原本。再取得は同じ ID + 版で重複を防ぎ、修正は別の版として残す。
-- source_revision は provider の版、または adapter が計算した内容 hash。
CREATE TABLE watch_sleep_observations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    device_id uuid NOT NULL,
    external_record_id text NOT NULL CHECK (btrim(external_record_id) <> ''),
    source_revision text NOT NULL CHECK (btrim(source_revision) <> ''),
    sleep_kind text CHECK (sleep_kind IN ('main', 'nap')), -- 不明なら null
    started_at timestamptz NOT NULL,
    ended_at timestamptz NOT NULL CHECK (ended_at > started_at),
    timezone text NOT NULL,
    duration_minutes integer CHECK (duration_minutes >= 0),
    source_deleted boolean NOT NULL DEFAULT false,
    raw_data jsonb NOT NULL CHECK (jsonb_typeof(raw_data) = 'object'),
    imported_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, user_id),
    UNIQUE (device_id, external_record_id, source_revision),
    FOREIGN KEY (device_id, user_id) REFERENCES watch_devices(id, user_id)
);
CREATE INDEX watch_sleep_observations_user_time ON watch_sleep_observations(user_id, started_at);

-- kind=prompt の例: agent.loop / job.extract_sleep / job.evaluate_sleep / job.plan_reminder / persona.arisu
-- loop の指示も本文として管理し、使用版は job_config_versions で固定する。
CREATE TABLE agent_config_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    config_key text NOT NULL CHECK (btrim(config_key) <> ''),
    kind text NOT NULL CHECK (kind IN ('prompt', 'skill')),
    version integer NOT NULL CHECK (version > 0),
    body text NOT NULL CHECK (btrim(body) <> ''),
    is_active boolean NOT NULL DEFAULT false,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (config_key, version),
    UNIQUE (id, config_key)
);
CREATE UNIQUE INDEX agent_config_one_active ON agent_config_versions(config_key) WHERE is_active;
-- 本文は上書きせず新版を追加。有効版の変更だけ UPDATE する。

CREATE TABLE jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    kind text NOT NULL CHECK (kind IN ('extract_sleep', 'evaluate_sleep', 'plan_reminder')),
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'cancelled')),
    dedup_key text NOT NULL, -- user 内で一意な投入キー。承認版・通知予定などから作る
    upload_id uuid,
    input jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(input) = 'object'),
    model text NOT NULL CHECK (btrim(model) <> ''),
    available_at timestamptz NOT NULL DEFAULT now(),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    lease_token uuid,
    lease_expires_at timestamptz,
    cancel_requested_at timestamptz,
    deleted_at timestamptz, -- タスク一覧からの削除。確定記録の出典は保持する。
    invalidated_at timestamptz, -- 参照記録が訂正・削除された評価。control が設定する
    started_at timestamptz,
    finished_at timestamptz,
    error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, user_id),
    UNIQUE (user_id, dedup_key),
    FOREIGN KEY (upload_id, user_id) REFERENCES uploads(id, user_id),
    CHECK ((kind = 'extract_sleep') = (upload_id IS NOT NULL)),
    CHECK ((status IN ('succeeded', 'failed', 'cancelled')) = (finished_at IS NOT NULL)),
    CHECK (status <> 'running' OR (lease_token IS NOT NULL AND lease_expires_at IS NOT NULL))
);
CREATE INDEX jobs_pending ON jobs(available_at) WHERE status = 'pending';
CREATE INDEX jobs_expired_lease ON jobs(lease_expires_at) WHERE status = 'running';
CREATE INDEX jobs_user_history ON jobs(user_id, created_at DESC);
-- worker は条件付き UPDATE で claim。結果確定時も lease_token と中止要求を照合する。

-- control が実行前に固定する。再試行でも同じ版を使う。
CREATE TABLE job_config_versions (
    job_id uuid NOT NULL REFERENCES jobs(id),
    config_key text NOT NULL,
    config_version_id uuid NOT NULL,
    PRIMARY KEY (job_id, config_key),
    FOREIGN KEY (config_version_id, config_key) REFERENCES agent_config_versions(id, config_key)
);

CREATE TABLE job_results (
    job_id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    output jsonb NOT NULL CHECK (jsonb_typeof(output) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (job_id, user_id) REFERENCES jobs(id, user_id)
);
-- extract_sleep の output は候補配列。配列 index を以下の source_candidate_index に使う。
-- 未承認の候補は sleep_records に自動反映しない。

-- 1 つの睡眠区間を id で識別し、訂正・削除時は revision を追加する。
-- 却下した画像候補も残す。主睡眠 / 昼寝は同日に複数件持てる。
CREATE TABLE sleep_records (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    user_id uuid NOT NULL REFERENCES users(id),
    is_current boolean NOT NULL DEFAULT true,
    status text NOT NULL CHECK (status IN ('approved', 'rejected', 'deleted')),
    sleep_kind text NOT NULL CHECK (sleep_kind IN ('main', 'nap')),
    sleep_date date NOT NULL, -- user が確認した現地日付。主睡眠は起床日を既定とする
    timezone text NOT NULL,
    started_at timestamptz,
    ended_at timestamptz,
    duration_minutes integer CHECK (duration_minutes >= 0),
    source text NOT NULL CHECK (source IN ('image', 'manual', 'watch')),
    source_watch_observation_id uuid,
    source_job_id uuid,
    source_candidate_index integer CHECK (source_candidate_index >= 0),
    submission_id uuid NOT NULL, -- 承認・手入力・訂正 request の冪等キー
    confirmed_at timestamptz NOT NULL DEFAULT now(), -- 本人の承認 / 却下 / 削除の日時
    PRIMARY KEY (id, revision),
    UNIQUE (id, revision, user_id),
    UNIQUE (user_id, submission_id),
    FOREIGN KEY (source_job_id, user_id) REFERENCES jobs(id, user_id),
    FOREIGN KEY (source_watch_observation_id, user_id) REFERENCES watch_sleep_observations(id, user_id),
    CHECK ((source = 'image' AND source_job_id IS NOT NULL AND source_candidate_index IS NOT NULL AND source_watch_observation_id IS NULL)
        OR (source = 'manual' AND source_job_id IS NULL AND source_candidate_index IS NULL AND source_watch_observation_id IS NULL)
        OR (source = 'watch' AND source_job_id IS NULL AND source_candidate_index IS NULL AND source_watch_observation_id IS NOT NULL)),
    CHECK (started_at IS NULL OR ended_at IS NULL OR ended_at > started_at),
    CHECK (started_at IS NULL OR ended_at IS NULL OR duration_minutes IS NULL
        OR duration_minutes * interval '1 minute' <= ended_at - started_at),
    CHECK (status <> 'approved' OR started_at IS NOT NULL OR ended_at IS NOT NULL OR duration_minutes IS NOT NULL)
);
CREATE UNIQUE INDEX sleep_records_one_current ON sleep_records(id) WHERE is_current;
CREATE UNIQUE INDEX sleep_records_candidate_once
    ON sleep_records(source_job_id, source_candidate_index) WHERE revision = 1 AND source = 'image';
CREATE UNIQUE INDEX sleep_records_watch_once
    ON sleep_records(source_watch_observation_id) WHERE revision = 1 AND source = 'watch';
CREATE INDEX sleep_records_user_date ON sleep_records(user_id, sleep_date DESC)
    WHERE is_current AND status = 'approved';
-- 訂正は旧版の is_current=false と新版 INSERT を同一 transaction で行う。
-- 元候補と修正値は job_results とこの表で辿る。分析は現行 approved だけを読む。

-- 画像 / 手入力の記録と watch の原本を関連付ける。候補を自動作成しても承認済み値は上書きしない。
CREATE TABLE sleep_watch_links (
    user_id uuid NOT NULL REFERENCES users(id),
    sleep_record_id uuid NOT NULL,
    sleep_record_revision integer NOT NULL,
    observation_id uuid NOT NULL,
    match_method text NOT NULL CHECK (match_method IN ('time_overlap', 'manual')),
    status text NOT NULL DEFAULT 'suggested' CHECK (status IN ('suggested', 'confirmed', 'rejected')),
    reviewed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (sleep_record_id, sleep_record_revision, observation_id),
    FOREIGN KEY (sleep_record_id, sleep_record_revision, user_id) REFERENCES sleep_records(id, revision, user_id),
    FOREIGN KEY (observation_id, user_id) REFERENCES watch_sleep_observations(id, user_id),
    CHECK ((status IN ('confirmed', 'rejected')) = (reviewed_at IS NOT NULL))
);
CREATE INDEX sleep_watch_links_observation ON sleep_watch_links(observation_id);
-- 記録を訂正したら新版に対して再照合する。関連付けの承認と睡眠記録の承認は別。

CREATE TABLE pvt_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), -- 同じ送信 ID の再送は重複登録しない
    user_id uuid NOT NULL REFERENCES users(id),
    started_at timestamptz NOT NULL,
    finished_at timestamptz NOT NULL CHECK (finished_at > started_at),
    timezone text NOT NULL,
    device text NOT NULL,
    input_method text NOT NULL,
    protocol_version text NOT NULL DEFAULT 'pvt-1m-v1',
    trials jsonb NOT NULL CHECK (jsonb_typeof(trials) = 'array'),
    -- trial: {stimulus_at_ms, response_at_ms, rt_ms, false_start}。刺激前は stimulus_at_ms=null。
    -- 生データを残し、集計は control が計算する。中断した測定は completed=false。
    completed boolean NOT NULL DEFAULT true,
    median_rt_ms double precision CHECK (median_rt_ms >= 0 AND median_rt_ms < 'Infinity'::float8),
    lapses integer CHECK (lapses >= 0),
    slowest10_mean_ms double precision CHECK (slowest10_mean_ms >= 0 AND slowest10_mean_ms < 'Infinity'::float8),
    false_starts integer CHECK (false_starts >= 0),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX pvt_sessions_user_time ON pvt_sessions(user_id, started_at DESC);

-- 評価 job と参照した確定記録の版を結ぶ。訂正時に旧評価を無効化するために使う。
CREATE TABLE job_sleep_records (
    job_id uuid NOT NULL,
    user_id uuid NOT NULL,
    sleep_record_id uuid NOT NULL,
    sleep_record_revision integer NOT NULL,
    PRIMARY KEY (job_id, sleep_record_id),
    FOREIGN KEY (job_id, user_id) REFERENCES jobs(id, user_id),
    FOREIGN KEY (sleep_record_id, sleep_record_revision, user_id)
        REFERENCES sleep_records(id, revision, user_id)
);
CREATE INDEX job_sleep_records_source ON job_sleep_records(sleep_record_id, sleep_record_revision);

-- 配信は control が担う。decision job の結果には「送らない」判断も保存する。
CREATE TABLE notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    decision_job_id uuid,
    dedup_key text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('bedtime', 'pvt', 'feedback')),
    intensity text NOT NULL DEFAULT 'normal' CHECK (intensity IN ('normal', 'concern', 'strong')),
    body text NOT NULL,
    scheduled_at timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sending', 'sent', 'failed', 'cancelled')),
    sent_at timestamptz,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, dedup_key),
    FOREIGN KEY (decision_job_id, user_id) REFERENCES jobs(id, user_id),
    CHECK ((status = 'sent') = (sent_at IS NOT NULL))
);
CREATE INDEX notifications_due ON notifications(scheduled_at) WHERE status = 'pending';
CREATE INDEX notifications_user_history ON notifications(user_id, created_at DESC);
-- 外部送信との exactly-once は DB 制約だけでは保証しない。sending の復旧は送信側で扱う。

GRANT USAGE ON SCHEMA public TO suyasuya_control, suyasuya_agent;
GRANT SELECT, INSERT, UPDATE, DELETE ON
    users, app_sessions, user_settings, notification_destinations, uploads, agent_config_versions,
    jobs, job_config_versions, job_results, sleep_records, pvt_sessions,
    job_sleep_records, notifications, watch_devices, watch_sleep_observations, sleep_watch_links TO suyasuya_control;

GRANT SELECT ON user_settings, uploads, agent_config_versions, jobs, job_config_versions,
    job_results, sleep_records, pvt_sessions, job_sleep_records, notifications TO suyasuya_agent;
GRANT INSERT ON job_results TO suyasuya_agent;
GRANT SELECT ON watch_sleep_observations, sleep_watch_links TO suyasuya_agent;
GRANT UPDATE (status, attempts, lease_token, lease_expires_at, started_at, finished_at, error)
    ON jobs TO suyasuya_agent;
-- agent は認証情報を読めず、睡眠・PVT・設定・送信先を書き換えられない。

COMMIT;
