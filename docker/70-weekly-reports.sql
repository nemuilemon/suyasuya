BEGIN;
ALTER TABLE user_settings ADD COLUMN IF NOT EXISTS weekly_report_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE user_settings ADD COLUMN IF NOT EXISTS weekly_report_weekday smallint NOT NULL DEFAULT 1 CHECK (weekly_report_weekday BETWEEN 0 AND 6);
ALTER TABLE user_settings ADD COLUMN IF NOT EXISTS weekly_report_time time NOT NULL DEFAULT '09:00';
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('bedtime','pvt','feedback','weekly_report'));
COMMIT;
