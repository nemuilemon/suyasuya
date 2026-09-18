-- schema.sql 適用済みのテスト DB に管理者で実行する。データは残さない。
-- psql -v ON_ERROR_STOP=1 -d suyasuya -f tests/schema.sql
BEGIN;
DO $$
DECLARE
    u uuid := gen_random_uuid();
    other_user uuid := gen_random_uuid();
    image_id uuid;
    test_job_id uuid;
    record_id uuid := gen_random_uuid();
    request_id uuid := gen_random_uuid();
    watch_id uuid;
    observation_id uuid;
BEGIN
    INSERT INTO users(id, username, password_hash) VALUES
        (u, 'schema-test-' || u, 'test-hash'),
        (other_user, 'schema-test-' || other_user, 'test-hash');
    INSERT INTO user_settings(user_id, pvt_reminder_time) VALUES (u, '14:00');
    INSERT INTO uploads(user_id, content_type, bucket, object_name, expected_size_bytes, expires_at)
        VALUES (u, 'image/png', 'test-bucket', 'uploads/' || u, 100, now() + interval '15 minutes')
        RETURNING id INTO image_id;
    BEGIN
        UPDATE uploads SET status = 'uploaded' WHERE id = image_id;
        RAISE EXCEPTION 'GCS の実体情報なしで upload が確定した';
    EXCEPTION WHEN check_violation THEN NULL;
    END;
    UPDATE uploads SET status = 'uploaded', size_bytes = 100, generation = 1, uploaded_at = now()
        WHERE id = image_id;
    INSERT INTO jobs(user_id, kind, dedup_key, upload_id, model)
        VALUES (u, 'extract_sleep', 'image-1', image_id, 'test-model') RETURNING id INTO test_job_id;

    BEGIN
        INSERT INTO jobs(user_id, kind, dedup_key, upload_id, model)
            VALUES (other_user, 'extract_sleep', 'wrong-owner', image_id, 'test-model');
        RAISE EXCEPTION '他人の画像への参照が通った';
    EXCEPTION WHEN foreign_key_violation THEN NULL;
    END;

    INSERT INTO job_results VALUES (test_job_id, u, '{"candidates":[]}', now());
    INSERT INTO job_results VALUES (test_job_id, u, '{}', now()) ON CONFLICT DO NOTHING;
    IF (SELECT count(*) FROM job_results r WHERE r.job_id = test_job_id) <> 1 THEN
        RAISE EXCEPTION '結果の冪等性が壊れている';
    END IF;

    INSERT INTO sleep_records(id, user_id, status, sleep_kind, sleep_date, timezone,
        duration_minutes, source, submission_id)
        VALUES (record_id, u, 'approved', 'nap', '2026-09-16', 'Asia/Tokyo', 20, 'manual', request_id);
    INSERT INTO sleep_records(user_id, status, sleep_kind, sleep_date, timezone,
        duration_minutes, source, submission_id)
        VALUES (u, 'approved', 'nap', '2026-09-16', 'Asia/Tokyo', 15, 'manual', gen_random_uuid());

    BEGIN
        INSERT INTO sleep_records(user_id, status, sleep_kind, sleep_date, timezone,
            duration_minutes, source, submission_id)
            VALUES (u, 'approved', 'nap', '2026-09-16', 'Asia/Tokyo', 20, 'manual', request_id);
        RAISE EXCEPTION '同じ承認 request が重複保存された';
    EXCEPTION WHEN unique_violation THEN NULL;
    END;

    INSERT INTO watch_devices(user_id, provider) VALUES (u, 'test-adapter') RETURNING id INTO watch_id;
    INSERT INTO watch_sleep_observations(user_id, device_id, external_record_id, source_revision,
        sleep_kind, started_at, ended_at, timezone, raw_data)
        VALUES (u, watch_id, 'nap-1', 'v1', 'nap', '2026-09-16 14:00+09',
            '2026-09-16 14:20+09', 'Asia/Tokyo', '{}') RETURNING id INTO observation_id;
    BEGIN
        INSERT INTO watch_sleep_observations(user_id, device_id, external_record_id, source_revision,
            started_at, ended_at, timezone, raw_data)
            VALUES (u, watch_id, 'nap-1', 'v1', '2026-09-16 14:00+09',
                '2026-09-16 14:20+09', 'Asia/Tokyo', '{}');
        RAISE EXCEPTION 'watch の同じ原本が重複保存された';
    EXCEPTION WHEN unique_violation THEN NULL;
    END;
    BEGIN
        INSERT INTO sleep_watch_links(user_id, sleep_record_id, sleep_record_revision, observation_id, match_method)
            VALUES (other_user, record_id, 1, observation_id, 'manual');
        RAISE EXCEPTION '他人の watch 関連付けが通った';
    EXCEPTION WHEN foreign_key_violation THEN NULL;
    END;
    INSERT INTO sleep_watch_links(user_id, sleep_record_id, sleep_record_revision, observation_id, match_method)
        VALUES (u, record_id, 1, observation_id, 'time_overlap');
    BEGIN
        INSERT INTO sleep_records(user_id, status, sleep_kind, sleep_date, timezone,
            duration_minutes, source, source_watch_observation_id, submission_id)
            VALUES (other_user, 'approved', 'nap', '2026-09-16', 'Asia/Tokyo',
                20, 'watch', observation_id, gen_random_uuid());
        RAISE EXCEPTION '他人の watch 原本を確定記録に使えた';
    EXCEPTION WHEN foreign_key_violation THEN NULL;
    END;

    UPDATE sleep_records SET is_current = false WHERE id = record_id;
    INSERT INTO sleep_records(id, revision, user_id, status, sleep_kind, sleep_date,
        timezone, source, submission_id)
        VALUES (record_id, 2, u, 'deleted', 'nap', '2026-09-16', 'Asia/Tokyo', 'manual', gen_random_uuid());
    IF (SELECT count(*) FROM sleep_records WHERE id = record_id) <> 2 THEN
        RAISE EXCEPTION '削除前の履歴が失われた';
    END IF;

    IF has_table_privilege('suyasuya_agent', 'sleep_records', 'INSERT')
        OR has_table_privilege('suyasuya_agent', 'watch_sleep_observations', 'INSERT')
        OR has_table_privilege('suyasuya_agent', 'agent_config_versions', 'UPDATE')
        OR has_table_privilege('suyasuya_agent', 'notification_destinations', 'SELECT')
        OR has_table_privilege('suyasuya_agent', 'users', 'SELECT')
        OR has_table_privilege('suyasuya_agent', 'app_sessions', 'SELECT')
        OR has_column_privilege('suyasuya_agent', 'jobs', 'user_id', 'UPDATE')
        OR has_schema_privilege('suyasuya_agent', 'public', 'CREATE') THEN
        RAISE EXCEPTION 'agent の権限が過大';
    END IF;
    IF NOT has_table_privilege('suyasuya_agent', 'job_results', 'INSERT')
        OR NOT has_column_privilege('suyasuya_agent', 'jobs', 'status', 'UPDATE') THEN
        RAISE EXCEPTION 'agent の実行権限が不足';
    END IF;
END
$$;
ROLLBACK;
