package repository

import (
	"context"
	"encoding/json"
	"suyasuya/internal/domain"
	"time"
)

func (q *Queries) SleepSamples(ctx context.Context, userID, start, end string) ([]domain.SleepSample, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT id,revision,sleep_date::text,sleep_kind,duration_minutes,started_at FROM sleep_records WHERE user_id=$1 AND is_current AND status='approved' AND sleep_date BETWEEN $2::date AND $3::date ORDER BY sleep_date,started_at FOR SHARE`, userID, start, end)
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
	return out, rows.Err()
}
func (q *Queries) UserTimezone(ctx context.Context, userID string) (string, error) {
	var tz string
	err := q.db.QueryRowContext(ctx, `SELECT timezone FROM user_settings WHERE user_id=$1`, userID).Scan(&tz)
	return tz, err
}
func (q *Queries) WeeklySchedules(ctx context.Context) ([]domain.WeeklySchedule, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT user_id,timezone,to_char(weekly_report_time,'HH24:MI'),weekly_report_weekday,snoozed_until FROM user_settings JOIN notification_destinations USING(user_id) WHERE notifications_enabled AND weekly_report_enabled`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.WeeklySchedule{}
	for rows.Next() {
		var s domain.WeeklySchedule
		if err = rows.Scan(&s.UserID, &s.Timezone, &s.Target, &s.Weekday, &s.SnoozedUntil); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func (q *Queries) SaveWeeklySettings(ctx context.Context, userID string, enabled *bool, weekday *int, clock *string) error {
	_, err := q.db.ExecContext(ctx, `UPDATE user_settings SET weekly_report_enabled=coalesce($2,weekly_report_enabled),weekly_report_weekday=coalesce($3,weekly_report_weekday),weekly_report_time=coalesce($4::time,weekly_report_time) WHERE user_id=$1`, userID, enabled, weekday, clock)
	if err != nil {
		return err
	}
	if enabled != nil && !*enabled {
		_, err = q.db.ExecContext(ctx, `UPDATE jobs SET cancel_requested_at=now(),status='cancelled',finished_at=now(),lease_token=NULL,lease_expires_at=NULL WHERE user_id=$1 AND kind='weekly_report' AND input->>'automatic'='true' AND status IN ('pending','running')`, userID)
	}
	return err
}
func (q *Queries) CreateWeeklyJob(ctx context.Context, userID, dedup, model, input string) (string, error) {
	var id string
	err := q.db.QueryRowContext(ctx, `WITH active AS MATERIALIZED (
 SELECT id,config_key FROM agent_config_versions WHERE config_key='job.weekly_report' AND user_id=$1 AND is_active
 ), created AS (
 INSERT INTO jobs(user_id,kind,dedup_key,model,input) SELECT $1,'weekly_report',$2,$3,$4::jsonb WHERE EXISTS(SELECT 1 FROM active)
 ON CONFLICT(user_id,dedup_key) DO UPDATE SET dedup_key=excluded.dedup_key RETURNING id
 ), bound AS (
 INSERT INTO job_config_versions(job_id,config_key,config_version_id) SELECT c.id,a.config_key,a.id FROM created c CROSS JOIN active a ON CONFLICT DO NOTHING
 ) SELECT id FROM created`, userID, dedup, model, input).Scan(&id)
	return id, err
}
func (q *Queries) BindWeeklyRecords(ctx context.Context, jobID, userID string) error {
	_, err := q.db.ExecContext(ctx, `INSERT INTO job_sleep_records(job_id,user_id,sleep_record_id,sleep_record_revision)
 SELECT j.id,j.user_id,(s->>'id')::uuid,(s->>'revision')::int FROM jobs j CROSS JOIN LATERAL jsonb_array_elements(j.input->'report'->'samples') s WHERE j.id=$1 AND j.user_id=$2 ON CONFLICT DO NOTHING`, jobID, userID)
	return err
}
func (q *Queries) WeeklyJobExists(ctx context.Context, userID, dedup string) (bool, error) {
	var exists bool
	err := q.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE user_id=$1 AND dedup_key=$2)`, userID, dedup).Scan(&exists)
	return exists, err
}
func (q *Queries) WeeklyPrompt(ctx context.Context, id, userID string) (string, error) {
	var s string
	err := q.db.QueryRowContext(ctx, `SELECT c.body FROM job_config_versions v JOIN agent_config_versions c ON c.id=v.config_version_id JOIN jobs j ON j.id=v.job_id WHERE j.id=$1 AND j.user_id=$2 AND c.user_id=j.user_id AND v.config_key='job.weekly_report'`, id, userID).Scan(&s)
	return s, err
}
func (q *Queries) Reports(ctx context.Context, userID string) ([]json.RawMessage, error) {
	return jsonRows(ctx, q.db, `SELECT row_to_json(x) FROM (SELECT j.id,j.status,j.error,j.created_at,j.invalidated_at,j.input->'report' AS report,r.output,n.status AS delivery_status FROM jobs j LEFT JOIN job_results r ON r.job_id=j.id AND r.user_id=j.user_id LEFT JOIN notifications n ON n.decision_job_id=j.id AND n.user_id=j.user_id WHERE j.user_id=$1 AND j.kind='weekly_report' AND j.deleted_at IS NULL ORDER BY j.created_at DESC LIMIT 30) x`, userID)
}
func (q *Queries) UnsentWeeklyReports(ctx context.Context) ([]PlannedReminder, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT j.id,j.user_id,j.dedup_key,j.input,r.output FROM jobs j JOIN job_results r ON r.job_id=j.id AND r.user_id=j.user_id WHERE j.kind='weekly_report' AND j.status='succeeded' AND j.input->>'deliver'='true' AND NOT EXISTS(SELECT 1 FROM notifications n WHERE n.user_id=j.user_id AND n.dedup_key=j.dedup_key) LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlannedReminder{}
	for rows.Next() {
		var p PlannedReminder
		if err = rows.Scan(&p.ID, &p.UserID, &p.Dedup, &p.Input, &p.Output); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (q *Queries) CreateWeeklyNotification(ctx context.Context, p PlannedReminder, body string, due time.Time) error {
	_, err := q.db.ExecContext(ctx, `INSERT INTO notifications(user_id,decision_job_id,dedup_key,kind,body,scheduled_at,expires_at) VALUES($1,$2,$3,'weekly_report',$4,$5,$5::timestamptz+interval '24 hours') ON CONFLICT(user_id,dedup_key) DO NOTHING`, p.UserID, p.ID, p.Dedup, body, due)
	return err
}

func (q *Queries) RecentWeeklyCount(ctx context.Context, userID string, count *int) error {
	return q.db.QueryRowContext(ctx, `SELECT count(*) FROM jobs WHERE user_id=$1 AND kind='weekly_report' AND created_at>now()-interval '1 minute'`, userID).Scan(count)
}
func (q *Queries) WeeklyEnabled(ctx context.Context, userID string) (bool, error) {
	var enabled bool
	err := q.db.QueryRowContext(ctx, `SELECT weekly_report_enabled FROM user_settings WHERE user_id=$1`, userID).Scan(&enabled)
	return enabled, err
}
