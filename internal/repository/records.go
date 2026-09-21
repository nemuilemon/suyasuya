package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

func (q *Queries) Records(ctx context.Context, userID string) ([]json.RawMessage, error) {
	return jsonRows(ctx, q.db, `SELECT row_to_json(s) FROM (SELECT * FROM sleep_records WHERE user_id=$1 AND is_current AND status='approved' ORDER BY sleep_date DESC,started_at DESC NULLS LAST LIMIT 200) s`, userID)
}

func (q *Queries) History(ctx context.Context, id string, userID string) ([]json.RawMessage, error) {
	return jsonRows(ctx, q.db, `SELECT row_to_json(s) FROM (SELECT * FROM sleep_records WHERE id=$1 AND user_id=$2 ORDER BY revision DESC) s`, id, userID)
}

func (q *Queries) LockSubmission(ctx context.Context, lockKey string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, lockKey)
}

func (q *Queries) SubmissionRecord(ctx context.Context, userID string, submissionID string, dstExisting *string) error {
	return q.db.QueryRowContext(ctx, `SELECT id FROM sleep_records WHERE user_id=$1 AND submission_id=$2`, userID, submissionID).Scan(dstExisting)
}

func (q *Queries) CurrentRecord(ctx context.Context, id string, userID string, dstRev *int, dstSource *string, dstJob *sql.NullString, dstCandidate *sql.NullInt64, dstWatch *sql.NullString, dstCurrentStatus *string) error {
	return q.db.QueryRowContext(ctx, `SELECT revision,source,source_job_id,source_candidate_index,source_watch_observation_id,status FROM sleep_records WHERE id=$1 AND user_id=$2 AND is_current FOR UPDATE`, id, userID).Scan(dstRev, dstSource, dstJob, dstCandidate, dstWatch, dstCurrentStatus)
}

func (q *Queries) RetireRecord(ctx context.Context, id string, rev int, userID string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `UPDATE sleep_records SET is_current=false WHERE id=$1 AND revision=$2 AND user_id=$3`, id, rev, userID)
}

func (q *Queries) CandidateOutput(ctx context.Context, jobID string, userID string, dstOutput *[]byte) error {
	return q.db.QueryRowContext(ctx, `SELECT r.output FROM job_results r JOIN jobs j ON j.id=r.job_id AND j.user_id=r.user_id WHERE j.id=$1 AND j.user_id=$2 AND j.kind='extract_sleep' AND j.status='succeeded' AND j.deleted_at IS NULL FOR UPDATE OF j`, jobID, userID).Scan(dstOutput)
}

func (q *Queries) CandidateReviewed(ctx context.Context, userID string, jobID string, candidateIndex int, dstAlready *bool) error {
	return q.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sleep_records WHERE user_id=$1 AND source_job_id=$2 AND source_candidate_index=$3)`, userID, jobID, candidateIndex).Scan(dstAlready)
}

func (q *Queries) InsertRecord(ctx context.Context, id string, rev int, userID string, status string, kind string, date string, timezone string, start *time.Time, end *time.Time, duration *int, source string, job sql.NullString, candidate sql.NullInt64, watch sql.NullString, submissionID string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `INSERT INTO sleep_records(id,revision,user_id,status,sleep_kind,sleep_date,timezone,started_at,ended_at,duration_minutes,source,source_job_id,source_candidate_index,source_watch_observation_id,submission_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, id, rev, userID, status, kind, date, timezone, start, end, duration, source, job, candidate, watch, submissionID)
}

func (q *Queries) InvalidateEvaluations(ctx context.Context, userID string, id string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `UPDATE jobs SET invalidated_at=now() WHERE user_id=$1 AND id IN (SELECT job_id FROM job_sleep_records WHERE user_id=$1 AND sleep_record_id=$2)`, userID, id)
}

func (q *Queries) Settings(ctx context.Context, userID string, dstB *[]byte) error {
	return q.db.QueryRowContext(ctx, `SELECT to_jsonb(s) || jsonb_build_object('webhook_configured',EXISTS(SELECT 1 FROM notification_destinations d WHERE d.user_id=s.user_id)) FROM user_settings s WHERE user_id=$1`, userID).Scan(dstB)
}

func (q *Queries) SaveSettings(ctx context.Context, userID string, timezone string, pvt string, bedtime string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `UPDATE user_settings SET timezone=$2,pvt_reminder_time=NULLIF($3,'')::time,target_bedtime=NULLIF($4,'')::time,updated_at=now() WHERE user_id=$1`, userID, timezone, pvt, bedtime)
}

func (q *Queries) Prompts(ctx context.Context, userID string) ([]json.RawMessage, error) {
	return jsonRows(ctx, q.db, `SELECT row_to_json(c) FROM (SELECT * FROM agent_config_versions WHERE user_id=$1 ORDER BY config_key,version DESC) c`, userID)
}

func (q *Queries) LockPrompt(ctx context.Context, key, userID string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, userID+":"+key)
}

func (q *Queries) RetirePrompt(ctx context.Context, key, userID string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `UPDATE agent_config_versions SET is_active=false WHERE config_key=$1 AND user_id=$2 AND is_active`, key, userID)
}

func (q *Queries) InsertPrompt(ctx context.Context, key string, body string, userID string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `INSERT INTO agent_config_versions(config_key,kind,version,body,is_active,created_by) SELECT $1,'prompt',coalesce(max(version),0)+1,$2,true,$3 FROM agent_config_versions WHERE config_key=$1 AND user_id=$3`, key, body, userID)
}
