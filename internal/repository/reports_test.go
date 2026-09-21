package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"suyasuya/internal/domain"
	"suyasuya/internal/platform"
	"testing"
	"time"
)

func TestWeeklyReportIntegration(t *testing.T) {
	name := os.Getenv("REMINDER_TEST_DB")
	if name == "" {
		t.Skip("disposable DB required")
	}
	uri := url.URL{Scheme: "postgres", Host: "127.0.0.1:5432", Path: "/" + name, User: url.UserPassword(os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD")), RawQuery: "sslmode=disable"}
	db, err := sql.Open("postgres", uri.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := New(db)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	u, other, record := platform.UUID(), platform.UUID(), platform.UUID()
	defer cleanupNotificationFixtures(t, db, u, other)
	exec(`INSERT INTO users(id,username,password_hash) VALUES($1::uuid,$1::text,'test'),($2::uuid,$2::text,'test')`, u, other)
	exec(`INSERT INTO user_settings(user_id,notifications_enabled,weekly_report_enabled) VALUES($1,true,true),($2,false,false)`, u, other)
	exec(`INSERT INTO notification_destinations(user_id,discord_webhook_url) VALUES($1,'https://discord.com/api/webhooks/1/test')`, u)
	exec(`INSERT INTO agent_config_versions(config_key,kind,version,body,is_active,created_by) VALUES('job.weekly_report','prompt',1,'weekly',true,$1)`, u)
	exec(`INSERT INTO sleep_records(id,user_id,status,sleep_kind,sleep_date,timezone,duration_minutes,source,submission_id) VALUES($1,$2,'approved','main','2026-09-14','Asia/Tokyo',420,'manual',$3),($4,$2,'rejected','nap','2026-09-14','Asia/Tokyo',30,'manual',$5)`, record, u, platform.UUID(), platform.UUID(), platform.UUID())
	records, err := repo.SleepSamples(ctx, u, "2026-09-14", "2026-09-20")
	if err != nil || len(records) != 1 {
		t.Fatalf("samples: %v %v", records, err)
	}
	foreign, err := repo.SleepSamples(ctx, other, "2026-09-14", "2026-09-20")
	if err != nil || len(foreign) != 0 {
		t.Fatal("sample isolation")
	}
	in := domain.WeeklyInput{Automatic: true, Deliver: true, Timezone: "Asia/Tokyo", Target: "09:00", Weekday: 1, DueAt: time.Now(), Report: domain.SummarizeSleep("2026-09-14", "2026-09-20", "2026-09-20", "Asia/Tokyo", records)}
	b, _ := json.Marshal(in)
	id, err := repo.CreateWeeklyJob(ctx, u, "weekly:2026-09-14", "test", string(b))
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := repo.CreateWeeklyJob(ctx, u, "weekly:2026-09-14", "test", string(b))
	if err != nil || duplicate != id {
		t.Fatal("weekly dedup")
	}
	if err = repo.BindWeeklyRecords(ctx, id, u); err != nil {
		t.Fatal(err)
	}
	prompt, err := repo.WeeklyPrompt(ctx, id, u)
	if err != nil || prompt != "weekly" {
		t.Fatal("prompt binding")
	}
	if _, err = repo.WeeklyPrompt(ctx, id, other); err == nil {
		t.Fatal("prompt isolation")
	}
	lease := platform.UUID()
	var task domain.Task
	if err = repo.ClaimTask(ctx, id, lease, &task); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.CompleteJob(ctx, id, u, lease); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.InsertResult(ctx, id, u, `{"body":"weekly summary"}`); err != nil {
		t.Fatal(err)
	}
	reports, err := repo.Reports(ctx, u)
	if err != nil || len(reports) != 1 {
		t.Fatal("report history")
	}
	reports, err = repo.Reports(ctx, other)
	if err != nil || len(reports) != 0 {
		t.Fatal("report isolation")
	}
	plans, err := repo.UnsentWeeklyReports(ctx)
	if err != nil || len(plans) != 1 {
		t.Fatal("publication queue")
	}
	if err = repo.CreateWeeklyNotification(ctx, plans[0], "summary", in.DueAt); err != nil {
		t.Fatal(err)
	}
	delivery, err := repo.ClaimDelivery(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if delivery.UserID != u {
		t.Fatal("wrong destination")
	}
	if err = repo.FinishDelivery(ctx, delivery.ID, u, "pending", "", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	// Editing an input record invalidates an unsent report.
	if _, err = repo.InvalidateEvaluations(ctx, u, record); err != nil {
		t.Fatal(err)
	}
	if err = repo.RecoverDeliveries(ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = db.QueryRow(`SELECT status FROM notifications WHERE id=$1`, delivery.ID).Scan(&status); err != nil || status != "cancelled" {
		t.Fatal("stale report sent")
	}
	// Weekly switch blocks an automatic publication. Manual generation itself has no notification.
	exec(`UPDATE jobs SET invalidated_at=NULL WHERE id=$1`, id)
	exec(`UPDATE notifications SET status='pending' WHERE id=$1`, delivery.ID)
	enabled := false
	if err = repo.SaveWeeklySettings(ctx, u, &enabled, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err = repo.RecoverDeliveries(ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT status FROM notifications WHERE id=$1`, delivery.ID).Scan(&status); err != nil || status != "cancelled" {
		t.Fatal("weekly off ignored")
	}
	// Explicit manual send remains possible when automatic notifications are off.
	exec(`UPDATE jobs SET input=jsonb_set(input,'{automatic}','false') WHERE id=$1`, id)
	exec(`UPDATE notifications SET status='pending' WHERE id=$1`, delivery.ID)
	if err = repo.SetNotificationsEnabled(ctx, u, false); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ClaimDelivery(ctx); err != nil {
		t.Fatal("manual send disabled with automatic switch", err)
	}
}

func cleanupNotificationFixtures(t *testing.T, db *sql.DB, u, other string) {
	t.Helper()
	for _, query := range []string{
		`DELETE FROM notifications WHERE user_id IN ($1,$2)`,
		`DELETE FROM job_sleep_records WHERE user_id IN ($1,$2)`,
		`DELETE FROM job_config_versions WHERE job_id IN (SELECT id FROM jobs WHERE user_id IN ($1,$2))`,
		`DELETE FROM job_results WHERE user_id IN ($1,$2)`,
		`DELETE FROM jobs WHERE user_id IN ($1,$2)`,
		`DELETE FROM sleep_records WHERE user_id IN ($1,$2)`,
		`DELETE FROM agent_config_versions WHERE created_by IN ($1,$2)`,
		`DELETE FROM notification_destinations WHERE user_id IN ($1,$2)`,
		`DELETE FROM user_settings WHERE user_id IN ($1,$2)`,
		`DELETE FROM users WHERE id IN ($1,$2)`,
	} {
		if _, err := db.Exec(query, u, other); err != nil {
			t.Error(err)
		}
	}
}
