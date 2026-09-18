package repository

import (
	"context"
	"database/sql"
)

func (q *Queries) SessionUser(ctx context.Context, tokenHash string, dstUserID *string, dstUsername *string, dstAdmin *bool) error {
	return q.db.QueryRowContext(ctx, `SELECT u.id,u.username,u.is_admin FROM app_sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now()`, tokenHash).Scan(dstUserID, dstUsername, dstAdmin)
}

func (q *Queries) EnsureAdmin(ctx context.Context, name string, passwordHash string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `INSERT INTO users(username,password_hash,is_admin) VALUES($1,$2,true) ON CONFLICT(username) DO NOTHING`, name, passwordHash)
}

func (q *Queries) UserID(ctx context.Context, name string, dstID *string) error {
	return q.db.QueryRowContext(ctx, `SELECT id FROM users WHERE username=$1`, name).Scan(dstID)
}

func (q *Queries) EnsureSettings(ctx context.Context, id string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `INSERT INTO user_settings(user_id) VALUES($1) ON CONFLICT DO NOTHING`, id)
}

func (q *Queries) EnsurePrompt(ctx context.Context, k string, body string, id string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `INSERT INTO agent_config_versions(config_key,kind,version,body,is_active,created_by) SELECT $1,'prompt',1,$2,true,$3 WHERE NOT EXISTS(SELECT 1 FROM agent_config_versions WHERE config_key=$1)`, k, body, id)
}

func (q *Queries) LoginCredentials(ctx context.Context, username string, dstID *string, dstHash *string) error {
	return q.db.QueryRowContext(ctx, `SELECT id,password_hash FROM users WHERE username=$1`, username).Scan(dstID, dstHash)
}

func (q *Queries) CreateSession(ctx context.Context, tokenHash string, id string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `INSERT INTO app_sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '7 days')`, tokenHash, id)
}

func (q *Queries) DeleteSession(ctx context.Context, tokenHash string, userID string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `DELETE FROM app_sessions WHERE token_hash=$1 AND user_id=$2`, tokenHash, userID)
}
