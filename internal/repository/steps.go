package repository

import (
	"context"
	"database/sql"
	"encoding/json"
)

// InsertStep appends one step to a run's trace. The row is only written while
// the run still holds the job's lease, so a run that was cancelled, deleted or
// recovered by another agent cannot keep adding to it.
func (q *Queries) InsertStep(ctx context.Context, jobID, userID, lease string, seq int, kind, name string, input, output []byte, ok bool, durationMS int64) error {
	res, err := q.db.ExecContext(ctx, `INSERT INTO job_steps(job_id,user_id,run_id,seq,kind,name,input,output,ok,duration_ms)
 SELECT j.id,j.user_id,$3,$4,$5,nullif($6,''),$7::jsonb,$8::jsonb,$9,$10 FROM jobs j
 WHERE j.id=$1 AND j.user_id=$2 AND j.lease_token=$3`,
		jobID, userID, lease, seq, kind, name, nullJSON(input), nullJSON(output), ok, durationMS)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func nullJSON(b []byte) any {
	if b == nil {
		return nil
	}
	return string(b)
}

// AgentRuns lists the user's recent tasks of every kind with how many steps
// their latest run recorded. The steps themselves are fetched per task.
func (q *Queries) AgentRuns(ctx context.Context, userID string) ([]json.RawMessage, error) {
	return jsonRows(ctx, q.db, `SELECT row_to_json(x) FROM (
 SELECT j.id,j.kind,j.status,j.error,j.model,j.attempts,j.created_at,j.started_at,j.finished_at,
  (SELECT count(*) FROM job_steps s WHERE s.job_id=j.id AND s.user_id=j.user_id) AS steps,
  (SELECT count(*) FROM job_steps s WHERE s.job_id=j.id AND s.user_id=j.user_id AND s.kind='tool') AS tool_calls
 FROM jobs j WHERE j.user_id=$1 AND j.deleted_at IS NULL
 ORDER BY j.created_at DESC LIMIT 50) x`, userID)
}

// JobSteps returns every recorded step of one of the user's tasks, grouped by
// run in the order the runs started.
func (q *Queries) JobSteps(ctx context.Context, jobID, userID string) ([]json.RawMessage, error) {
	return jsonRows(ctx, q.db, `SELECT row_to_json(x) FROM (
 SELECT s.run_id,s.seq,s.kind,s.name,s.input,s.output,s.ok,s.duration_ms,s.created_at
 FROM job_steps s JOIN jobs j ON j.id=s.job_id AND j.user_id=s.user_id
 WHERE s.job_id=$1 AND s.user_id=$2
 ORDER BY min(s.created_at) OVER (PARTITION BY s.run_id), s.run_id, s.seq) x`, jobID, userID)
}
