ALTER TABLE jobs ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
