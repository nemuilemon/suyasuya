package repository

import (
	"context"
	"encoding/json"
	"suyasuya/internal/domain"
	"time"
)

func (q *Queries) SetWebhook(ctx context.Context, userID, url string) error {
	if url == "" {
		_, err := q.db.ExecContext(ctx, `DELETE FROM notification_destinations WHERE user_id=$1`, userID)
		return err
	}
	_, err := q.db.ExecContext(ctx, `INSERT INTO notification_destinations(user_id,discord_webhook_url) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET discord_webhook_url=excluded.discord_webhook_url,updated_at=now()`, userID, url)
	return err
}

func (q *Queries) ReminderSchedules(ctx context.Context) ([]domain.ReminderSchedule, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT s.user_id,s.timezone,coalesce(to_char(s.target_bedtime,'HH24:MI'),''),coalesce(to_char(s.pvt_reminder_time,'HH24:MI'),''),s.snoozed_until FROM user_settings s JOIN notification_destinations d USING(user_id) WHERE s.notifications_enabled`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ReminderSchedule{}
	for rows.Next() {
		var s domain.ReminderSchedule
		if err = rows.Scan(&s.UserID, &s.Timezone, &s.Bedtime, &s.PVT, &s.SnoozedUntil); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func (q *Queries) ScheduleReminder(ctx context.Context, userID, dedup, model, input string) error {
	_, err := q.db.ExecContext(ctx, `WITH active AS MATERIALIZED (
 SELECT id,config_key FROM agent_config_versions WHERE is_active AND config_key IN ('agent.loop','job.plan_reminder')
 ), new_job AS (
 INSERT INTO jobs(user_id,kind,dedup_key,model,input)
 SELECT $1,'plan_reminder',$2,$3,$4::jsonb WHERE (SELECT count(*) FROM active)=2
 ON CONFLICT(user_id,dedup_key) DO NOTHING RETURNING id
 ) INSERT INTO job_config_versions(job_id,config_key,config_version_id)
 SELECT j.id,a.config_key,a.id FROM new_job j CROSS JOIN active a`, userID, dedup, model, input)
	return err
}
func (q *Queries) ReminderPrompt(ctx context.Context, id, userID string) (string, error) {
	var prompt string
	err := q.db.QueryRowContext(ctx, `SELECT string_agg(c.body,E'\n\n' ORDER BY v.config_key) FROM job_config_versions v JOIN agent_config_versions c ON c.id=v.config_version_id JOIN jobs j ON j.id=v.job_id WHERE j.id=$1 AND j.user_id=$2 AND v.config_key IN ('agent.loop','job.plan_reminder') HAVING count(*)=2`, id, userID).Scan(&prompt)
	return prompt, err
}

type PlannedReminder struct {
	ID, UserID, Dedup string
	Input, Output     []byte
}

func (q *Queries) PlannedReminders(ctx context.Context) ([]PlannedReminder, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT j.id,j.user_id,j.dedup_key,j.input,r.output FROM jobs j JOIN job_results r ON r.job_id=j.id AND r.user_id=j.user_id WHERE j.kind='plan_reminder' AND j.status='succeeded' AND j.deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM notifications n WHERE n.user_id=j.user_id AND n.dedup_key=j.dedup_key) ORDER BY j.created_at LIMIT 50`)
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
func (q *Queries) CreateReminderNotification(ctx context.Context, p PlannedReminder, in domain.ReminderInput, body string) error {
	_, err := q.db.ExecContext(ctx, `INSERT INTO notifications(user_id,decision_job_id,dedup_key,kind,body,scheduled_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$6::timestamptz+interval '15 minutes') ON CONFLICT(user_id,dedup_key) DO NOTHING`, p.UserID, p.ID, p.Dedup, in.Kind, body, in.DueAt)
	return err
}

// Stale/ineligible deliveries never survive disabling notifications or changing the target time.
const deliveryEligible = `EXISTS(SELECT 1 FROM user_settings s JOIN notification_destinations d USING(user_id)
 WHERE s.user_id=n.user_id AND (n.decision_job_id IS NULL OR
 (s.notifications_enabled AND (s.snoozed_until IS NULL OR s.snoozed_until<=now())
 AND EXISTS(SELECT 1 FROM jobs j WHERE j.id=n.decision_job_id AND j.user_id=n.user_id
 AND j.deleted_at IS NULL AND j.cancel_requested_at IS NULL AND j.invalidated_at IS NULL
 AND s.timezone=j.input->>'timezone'
 AND (CASE n.kind WHEN 'bedtime' THEN to_char(s.target_bedtime,'HH24:MI') WHEN 'pvt' THEN to_char(s.pvt_reminder_time,'HH24:MI') END)=j.input->>'target'))))`

func (q *Queries) RecoverDeliveries(ctx context.Context) error {
	_, err := q.db.ExecContext(ctx, `UPDATE notifications n SET status='cancelled',error='通知の期限切れ、または設定が変更されました' WHERE status='pending' AND (expires_at<=now() OR NOT (`+deliveryEligible+`))`)
	if err != nil {
		return err
	}
	_, err = q.db.ExecContext(ctx, `UPDATE notifications SET status='failed',error='送信中に接続が途切れました。重複防止のため自動再送しません' WHERE status='sending' AND sending_started_at<now()-interval '2 minutes'`)
	return err
}
func (q *Queries) ClaimDelivery(ctx context.Context) (domain.Delivery, error) {
	var d domain.Delivery
	err := q.db.QueryRowContext(ctx, `WITH candidate AS (
 SELECT n.id FROM notifications n WHERE n.status='pending' AND n.scheduled_at<=now() AND n.expires_at>now() AND `+deliveryEligible+`
 ORDER BY n.scheduled_at FOR UPDATE OF n SKIP LOCKED LIMIT 1
 ), claimed AS (UPDATE notifications n SET status='sending',attempts=attempts+1,sending_started_at=now(),error=NULL FROM candidate c WHERE n.id=c.id RETURNING n.*)
 SELECT n.id,n.user_id,d.discord_webhook_url,n.body,n.attempts FROM claimed n JOIN notification_destinations d USING(user_id)`).Scan(&d.ID, &d.UserID, &d.Webhook, &d.Body, &d.Attempts)
	return d, err
}
func (q *Queries) FinishDelivery(ctx context.Context, id, userID, status, messageID, message string, retryAt time.Time) error {
	_, err := q.db.ExecContext(ctx, `UPDATE notifications SET status=$3,discord_message_id=NULLIF($4,''),error=NULLIF($5,''),sent_at=CASE WHEN $3='sent' THEN now() ELSE NULL END,scheduled_at=CASE WHEN $3='pending' THEN $6 ELSE scheduled_at END WHERE id=$1 AND user_id=$2 AND status='sending'`, id, userID, status, messageID, message, retryAt)
	return err
}
func (q *Queries) SetNotificationsEnabled(ctx context.Context, userID string, enabled bool) error {
	_, err := q.db.ExecContext(ctx, `UPDATE user_settings SET notifications_enabled=$2,updated_at=now() WHERE user_id=$1`, userID, enabled)
	if err != nil {
		return err
	}
	if !enabled {
		_, err = q.db.ExecContext(ctx, `UPDATE notifications SET status='cancelled',error='自動通知を停止しました' WHERE user_id=$1 AND status='pending' AND decision_job_id IS NOT NULL`, userID)
		if err != nil {
			return err
		}
		_, err = q.db.ExecContext(ctx, `UPDATE jobs SET cancel_requested_at=now(),status='cancelled',finished_at=now(),lease_token=NULL,lease_expires_at=NULL WHERE user_id=$1 AND kind='plan_reminder' AND status IN ('pending','running')`, userID)
	}
	return err
}
func (q *Queries) NotificationHistory(ctx context.Context, userID string) ([]json.RawMessage, error) {
	return jsonRows(ctx, q.db, `SELECT row_to_json(x) FROM (
 SELECT id,kind,body,status,error,scheduled_at,sent_at,created_at FROM notifications WHERE user_id=$1
 UNION ALL
 SELECT j.id,j.input->>'kind','',CASE WHEN j.status='succeeded' THEN 'pending' ELSE j.status END,j.error,(j.input->>'due_at')::timestamptz,NULL,j.created_at FROM jobs j
 WHERE j.user_id=$1 AND j.kind='plan_reminder' AND NOT EXISTS(SELECT 1 FROM notifications n WHERE n.decision_job_id=j.id AND n.user_id=j.user_id)
 ORDER BY created_at DESC LIMIT 30) x`, userID)
}
func (q *Queries) CreateTestNotification(ctx context.Context, userID, id string) error {
	// The service holds the settings row lock. Repeated request IDs are successful no-ops.
	var exists bool
	if err := q.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM notifications WHERE user_id=$1 AND dedup_key='test:'||$2)`, userID, id).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	// Serialize per user so rapid concurrent clicks cannot flood Discord.
	tx, err := q.db.ExecContext(ctx, `INSERT INTO notifications(user_id,dedup_key,kind,body,scheduled_at,expires_at)
 SELECT $1,'test:'||$2,'feedback','接続テストだよ。睡眠の催促はこのチャンネルに届けるね。',now(),now()+interval '15 minutes'
 WHERE EXISTS(SELECT 1 FROM notification_destinations WHERE user_id=$1)
 AND NOT EXISTS(SELECT 1 FROM notifications WHERE user_id=$1 AND kind='feedback' AND created_at>now()-interval '1 minute')
 ON CONFLICT(user_id,dedup_key) DO NOTHING`, userID, id)
	if err != nil {
		return err
	}
	n, _ := tx.RowsAffected()
	if n == 0 {
		return domain.Invalid("Webhook を保存し、前の接続テストから1分空けてください")
	}
	return nil
}
func (q *Queries) LockNotificationSettings(ctx context.Context, userID string) error {
	var id string
	return q.db.QueryRowContext(ctx, `SELECT user_id FROM user_settings WHERE user_id=$1 FOR UPDATE`, userID).Scan(&id)
}

func (q *Queries) WebhookConfigured(ctx context.Context, userID string) (bool, error) {
	var configured bool
	err := q.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM notification_destinations WHERE user_id=$1)`, userID).Scan(&configured)
	return configured, err
}
