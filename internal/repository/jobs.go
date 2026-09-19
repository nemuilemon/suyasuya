package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/lib/pq"
)

func (q *Queries) RecoverExpiredJobs(ctx context.Context) (sql.Result, error) {
	return q.db.ExecContext(ctx, `UPDATE jobs SET status=CASE WHEN cancel_requested_at IS NOT NULL THEN 'cancelled' WHEN attempts>=3 THEN 'failed' ELSE 'pending' END,finished_at=CASE WHEN cancel_requested_at IS NOT NULL OR attempts>=3 THEN now() ELSE NULL END,lease_token=NULL,lease_expires_at=NULL WHERE kind='extract_sleep' AND status='running' AND lease_expires_at<now()`)
}

func (q *Queries) PendingJobs(ctx context.Context) ([]string, error) {
	rows, e := q.db.QueryContext(ctx, `SELECT id FROM jobs WHERE kind='extract_sleep' AND status='pending' AND available_at<=now() AND cancel_requested_at IS NULL LIMIT 100`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (q *Queries) DeleteExpiredSessions(ctx context.Context) (sql.Result, error) {
	return q.db.ExecContext(ctx, `DELETE FROM app_sessions WHERE expires_at<now()`)
}

func (q *Queries) ClaimJob(ctx context.Context, id string, lease string, dstUid *string, dstUpload *string, dstModel *string) error {
	return q.db.QueryRowContext(ctx, `UPDATE jobs SET status='running',attempts=attempts+1,lease_token=$2,lease_expires_at=now()+interval '3 minutes',started_at=now(),error=NULL WHERE id=$1 AND kind='extract_sleep' AND deleted_at IS NULL AND status='pending' AND available_at<=now() AND cancel_requested_at IS NULL RETURNING user_id,upload_id,model`, id, lease).Scan(dstUid, dstUpload, dstModel)
}

func (q *Queries) JobCancelled(ctx context.Context, id string, uid string, dstRequested *bool) error {
	return q.db.QueryRowContext(ctx, `SELECT cancel_requested_at IS NOT NULL FROM jobs WHERE id=$1 AND user_id=$2`, id, uid).Scan(dstRequested)
}

func (q *Queries) FailJob(ctx context.Context, id string, uid string, lease string, value4 string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `UPDATE jobs SET status=CASE WHEN cancel_requested_at IS NOT NULL THEN 'cancelled' ELSE 'failed' END,finished_at=now(),error=$4,lease_token=NULL,lease_expires_at=NULL WHERE id=$1 AND user_id=$2 AND lease_token=$3 AND status='running'`, id, uid, lease, value4)
}

func (q *Queries) CompleteJob(ctx context.Context, id string, uid string, lease string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `UPDATE jobs SET status='succeeded',finished_at=now(),lease_token=NULL,lease_expires_at=NULL WHERE id=$1 AND user_id=$2 AND lease_token=$3 AND status='running' AND cancel_requested_at IS NULL`, id, uid, lease)
}

func (q *Queries) CancelClaim(ctx context.Context, id string, uid string, lease string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `UPDATE jobs SET status='cancelled',finished_at=now(),lease_token=NULL,lease_expires_at=NULL WHERE id=$1 AND user_id=$2 AND lease_token=$3 AND cancel_requested_at IS NOT NULL`, id, uid, lease)
}

func (q *Queries) InsertResult(ctx context.Context, id string, uid string, output string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `INSERT INTO job_results(job_id,user_id,output) VALUES($1,$2,$3) ON CONFLICT(job_id) DO NOTHING`, id, uid, output)
}

func (q *Queries) JobPrompt(ctx context.Context, id string, uid string, dstPrompt *string) error {
	return q.db.QueryRowContext(ctx, `SELECT c.body FROM job_config_versions v JOIN agent_config_versions c ON c.id=v.config_version_id JOIN jobs j ON j.id=v.job_id WHERE v.job_id=$1 AND j.user_id=$2 AND v.config_key='job.extract_sleep'`, id, uid).Scan(dstPrompt)
}

func (q *Queries) Jobs(ctx context.Context, userID string) ([]json.RawMessage, error) {
	return jsonRows(ctx, q.db, `SELECT row_to_json(x) FROM (SELECT j.id,j.status,j.error,j.created_at,j.upload_id,u.capture_date,c.version AS prompt_version,r.output,coalesce((SELECT json_agg(source_candidate_index) FROM sleep_records WHERE user_id=j.user_id AND source_job_id=j.id AND revision=1),'[]') AS reviewed FROM jobs j JOIN uploads u ON u.id=j.upload_id AND u.user_id=j.user_id LEFT JOIN job_results r ON r.job_id=j.id AND r.user_id=j.user_id LEFT JOIN job_config_versions v ON v.job_id=j.id AND v.config_key='job.extract_sleep' LEFT JOIN agent_config_versions c ON c.id=v.config_version_id WHERE j.user_id=$1 AND j.kind='extract_sleep' AND j.deleted_at IS NULL ORDER BY j.created_at DESC LIMIT 50) x`, userID)
}

func (q *Queries) RetryJob(ctx context.Context, id string, userID string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `UPDATE jobs SET status='pending',attempts=0,finished_at=NULL,cancel_requested_at=NULL,error=NULL,available_at=now(),lease_token=NULL,lease_expires_at=NULL WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL AND status IN ('failed','cancelled')`, id, userID)
}

func (q *Queries) CancelJob(ctx context.Context, id string, userID string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `UPDATE jobs SET cancel_requested_at=now(),status=CASE WHEN status='pending' THEN 'cancelled' ELSE status END,finished_at=CASE WHEN status='pending' THEN now() ELSE finished_at END WHERE id=$1 AND user_id=$2 AND status IN ('pending','running')`, id, userID)
}

// DeleteJobs retains provenance for approved sleep records. Revoking active leases
// prevents an in-flight worker from publishing a result after deletion.
func (q *Queries) DeleteJobs(ctx context.Context, userID string, ids []string, completed bool) (int64, error) {
	result, err := q.db.ExecContext(ctx, `
 UPDATE jobs j SET deleted_at=now(),
  cancel_requested_at=CASE WHEN status IN ('pending','running') THEN now() ELSE cancel_requested_at END,
  finished_at=CASE WHEN status IN ('pending','running') THEN now() ELSE finished_at END,
  status=CASE WHEN status IN ('pending','running') THEN 'cancelled' ELSE status END,
  lease_token=NULL,lease_expires_at=NULL
 WHERE j.user_id=$1 AND j.kind='extract_sleep' AND j.deleted_at IS NULL
 AND (j.id=ANY($2::uuid[]) OR ($3 AND j.status='succeeded'
   AND EXISTS(SELECT 1 FROM job_results r WHERE r.job_id=j.id AND r.user_id=j.user_id
     AND jsonb_array_length(r.output->'records')>0
     AND jsonb_array_length(r.output->'records')=(SELECT count(DISTINCT s.source_candidate_index)
       FROM sleep_records s WHERE s.user_id=j.user_id AND s.source_job_id=j.id))))`, userID, pq.Array(ids), completed)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
