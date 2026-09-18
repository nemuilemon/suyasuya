-- 既存 DB 用。過去の測定の protocol_version は変更しない。
ALTER TABLE pvt_sessions ALTER COLUMN protocol_version SET DEFAULT 'pvt-1m-v1';
