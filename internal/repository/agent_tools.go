package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"suyasuya/internal/domain"
)

// Agent queries are SELECT-only: do not reuse UI queries with FOR SHARE or secrets.
func (q *Queries) AgentSleepSamples(ctx context.Context, userID, from, to string) ([]domain.SleepSample, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT id,revision,sleep_date::text,sleep_kind,duration_minutes,started_at FROM sleep_records WHERE user_id=$1 AND is_current AND status='approved' AND sleep_date BETWEEN $2::date AND $3::date ORDER BY sleep_date,started_at LIMIT 501`, userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.SleepSample{}
	for rows.Next() {
		var s domain.SleepSample
		if err = rows.Scan(&s.ID, &s.Revision, &s.Date, &s.Kind, &s.Minutes, &s.StartedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if len(out) > 500 {
		return nil, fmt.Errorf("too many sleep records")
	}
	return out, rows.Err()
}
func (q *Queries) AgentPVT(ctx context.Context, userID, from, to, tz string) ([]json.RawMessage, error) {
	return jsonRows(ctx, q.db, `SELECT row_to_json(x) FROM (SELECT started_at,completed,protocol_version,median_rt_ms,lapses,slowest10_mean_ms,false_starts FROM pvt_sessions WHERE user_id=$1 AND started_at>=($2::date::timestamp AT TIME ZONE $4) AND started_at<(($3::date+1)::timestamp AT TIME ZONE $4) ORDER BY started_at DESC LIMIT 101) x`, userID, from, to, tz)
}
func (q *Queries) AgentSettings(ctx context.Context, userID string) (json.RawMessage, error) {
	var out []byte
	err := q.db.QueryRowContext(ctx, `SELECT row_to_json(x) FROM (SELECT agent_name,timezone,target_bedtime,pvt_reminder_time,notifications_enabled,snoozed_until,weekly_report_enabled,weekly_report_weekday,weekly_report_time FROM user_settings WHERE user_id=$1) x`, userID).Scan(&out)
	return out, err
}

func (q *Queries) AgentNotificationHistory(ctx context.Context, userID string) ([]json.RawMessage, error) {
	return jsonRows(ctx, q.db, `SELECT row_to_json(x) FROM (SELECT kind,body,status,scheduled_at,sent_at FROM notifications WHERE user_id=$1 ORDER BY created_at DESC LIMIT 20) x`, userID)
}
