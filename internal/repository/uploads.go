package repository

import (
	"context"
	"database/sql"
	"time"
)

func (q *Queries) InsertUpload(ctx context.Context, id string, userID string, typeValue string, bucket string, object string, size int64, expires time.Time, date string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `INSERT INTO uploads(id,user_id,content_type,bucket,object_name,expected_size_bytes,expires_at,capture_date) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, userID, typeValue, bucket, object, size, expires, date)
}

func (q *Queries) UploadForUpdate(ctx context.Context, id string, userID string, dstBucket *string, dstObject *string, dstContentType *string, dstStatus *string, dstExpected *int64, dstExpires *time.Time) error {
	return q.db.QueryRowContext(ctx, `SELECT bucket,object_name,content_type,status,expected_size_bytes,expires_at FROM uploads WHERE id=$1 AND user_id=$2 FOR UPDATE`, id, userID).Scan(dstBucket, dstObject, dstContentType, dstStatus, dstExpected, dstExpires)
}

func (q *Queries) UploadJob(ctx context.Context, userID string, dedupKey string, dstJobID *string) error {
	return q.db.QueryRowContext(ctx, `SELECT id FROM jobs WHERE user_id=$1 AND dedup_key=$2`, userID, dedupKey).Scan(dstJobID)
}

func (q *Queries) CompleteUpload(ctx context.Context, id string, userID string, generation int64, size int64) (sql.Result, error) {
	return q.db.ExecContext(ctx, `UPDATE uploads SET status='uploaded',generation=$3,size_bytes=$4,uploaded_at=now() WHERE id=$1 AND user_id=$2`, id, userID, generation, size)
}

func (q *Queries) InsertOCRJob(ctx context.Context, jobID string, userID string, dedupKey string, id string, model string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `INSERT INTO jobs(id,user_id,kind,dedup_key,upload_id,model) VALUES($1,$2,'extract_sleep',$3,$4,$5)`, jobID, userID, dedupKey, id, model)
}

func (q *Queries) BindOCRPrompt(ctx context.Context, jobID string) (sql.Result, error) {
	return q.db.ExecContext(ctx, `INSERT INTO job_config_versions(job_id,config_key,config_version_id) SELECT $1,config_key,id FROM agent_config_versions WHERE config_key='job.extract_sleep' AND user_id=(SELECT user_id FROM jobs WHERE id=$1) AND is_active`, jobID)
}

func (q *Queries) ImageLocation(ctx context.Context, id string, userID string, dstBucket *string, dstObject *string, dstGeneration *int64) error {
	return q.db.QueryRowContext(ctx, `SELECT bucket,object_name,generation FROM uploads WHERE id=$1 AND user_id=$2 AND status='uploaded'`, id, userID).Scan(dstBucket, dstObject, dstGeneration)
}

func (q *Queries) ImageForOCR(ctx context.Context, upload string, uid string, dstBucket *string, dstObject *string, dstContentType *string, dstGeneration *int64) error {
	return q.db.QueryRowContext(ctx, `SELECT bucket,object_name,content_type,generation FROM uploads WHERE id=$1 AND user_id=$2 AND status='uploaded'`, upload, uid).Scan(dstBucket, dstObject, dstContentType, dstGeneration)
}
