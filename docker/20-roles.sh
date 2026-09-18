#!/bin/sh
set -eu
[ "$DB_CONTROL_USER" = suyasuya_control ] && [ "$DB_AGENT_USER" = suyasuya_agent ] || { echo 'DB role names must match schema.sql' >&2; exit 1; }
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<'SQL'
\getenv control_password DB_CONTROL_PASSWORD
\getenv agent_password DB_AGENT_PASSWORD
ALTER ROLE suyasuya_control LOGIN PASSWORD :'control_password';
ALTER ROLE suyasuya_agent LOGIN PASSWORD :'agent_password';
SQL
