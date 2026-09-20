BEGIN;
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS expires_at timestamptz;
UPDATE notifications SET expires_at=scheduled_at+interval '15 minutes' WHERE expires_at IS NULL;
ALTER TABLE notifications ALTER COLUMN expires_at SET NOT NULL;
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS sending_started_at timestamptz;
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS discord_message_id text;
UPDATE notifications SET sending_started_at=created_at WHERE status='sending' AND sending_started_at IS NULL;
COMMIT;
