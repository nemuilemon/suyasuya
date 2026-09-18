package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

func (q *Queries) PVTList(ctx context.Context, userID string) ([]json.RawMessage, error) {
	return jsonRows(ctx, q.db, `SELECT row_to_json(s) FROM (SELECT id,started_at,finished_at,completed,protocol_version,median_rt_ms,lapses,slowest10_mean_ms,false_starts FROM pvt_sessions WHERE user_id=$1 ORDER BY started_at DESC LIMIT 100) s`, userID)
}

func (q *Queries) InsertPVT(ctx context.Context, id string, userID string, started time.Time, finished time.Time, timezone string, device string, input string, trials string, completed bool, median *float64, lapses int, slow *float64, false int) (sql.Result, error) {
	return q.db.ExecContext(ctx, `INSERT INTO pvt_sessions(id,user_id,started_at,finished_at,timezone,device,input_method,trials,completed,protocol_version,median_rt_ms,lapses,slowest10_mean_ms,false_starts) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'pvt-1m-v1',$10,$11,$12,$13) ON CONFLICT(id) DO NOTHING`, id, userID, started, finished, timezone, device, input, trials, completed, median, lapses, slow, false)
}
