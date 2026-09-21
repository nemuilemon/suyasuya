package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"testing"
	"time"

	"suyasuya/internal/domain"
	"suyasuya/internal/platform"
)

// Run only against a disposable database initialized with schema.sql.
func TestReminderDeliveryIntegration(t *testing.T) {
	name := os.Getenv("REMINDER_TEST_DB")
	if name == "" {
		t.Skip("set REMINDER_TEST_DB to a disposable database")
	}
	uri := url.URL{Scheme: "postgres", Host: "127.0.0.1:5432", Path: "/" + name, User: url.UserPassword(os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD")), RawQuery: "sslmode=disable"}
	db, err := sql.Open("postgres", uri.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(4)
	ctx := context.Background()
	repo := New(db)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	uid, other := platform.UUID(), platform.UUID()
	defer cleanupNotificationFixtures(t, db, uid, other)
	exec(`INSERT INTO users(id,username,password_hash) VALUES($1::uuid,$1::text,'test'),($2::uuid,$2::text,'test')`, uid, other)
	exec(`INSERT INTO user_settings(user_id,notifications_enabled,target_bedtime) VALUES($1,true,'23:00'),($2,true,'23:00')`, uid, other)
	exec(`INSERT INTO notification_destinations(user_id,discord_webhook_url) VALUES($1,'https://discord.com/api/webhooks/1/token'),($2,'https://discord.com/api/webhooks/2/token')`, uid, other)
	exec(`INSERT INTO agent_config_versions(config_key,kind,version,body,is_active,created_by) VALUES('agent.loop','prompt',1,'loop',true,$1),('job.plan_reminder','prompt',1,'reminder',true,$1)`, uid)
	schedules, err := repo.ReminderSchedules(ctx)
	if err != nil || len(schedules) != 2 {
		t.Fatalf("schedules: %v %v", schedules, err)
	}
	input := domain.ReminderInput{Kind: "bedtime", Timezone: "Asia/Tokyo", Target: "23:00", DueAt: time.Now()}
	b, _ := json.Marshal(input)
	for i := 0; i < 2; i++ {
		if err = repo.ScheduleReminder(ctx, uid, "reminder:bedtime:today", "test", string(b)); err != nil {
			t.Fatal(err)
		}
	}
	var jobID string
	var count int
	if err = db.QueryRow(`SELECT id FROM jobs WHERE user_id=$1`, uid).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM jobs WHERE user_id=$1`, uid).Scan(&count); err != nil || count != 1 {
		t.Fatalf("dedup %d %v", count, err)
	}
	prompt, err := repo.ReminderPrompt(ctx, jobID, uid)
	if err != nil || prompt != "reminder" {
		t.Fatalf("bound prompt %q %v", prompt, err)
	}
	if _, err = repo.ReminderPrompt(ctx, jobID, other); err == nil {
		t.Fatal("cross-user prompt leak")
	}
	var task domain.Task
	lease := platform.UUID()
	if err = repo.ClaimTask(ctx, jobID, lease, &task); err != nil || task.Kind != "plan_reminder" {
		t.Fatalf("claim: %v", err)
	}
	if _, err = repo.CompleteJob(ctx, jobID, uid, lease); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.InsertResult(ctx, jobID, uid, `{"body":"寝る時間だよ"}`); err != nil {
		t.Fatal(err)
	}
	plans, err := repo.PlannedReminders(ctx)
	if err != nil || len(plans) != 1 {
		t.Fatalf("plans %v %v", plans, err)
	}
	for i := 0; i < 2; i++ {
		if err = repo.CreateReminderNotification(ctx, plans[0], input, "寝る時間だよ"); err != nil {
			t.Fatal(err)
		}
	}
	if err = repo.RecoverDeliveries(ctx); err != nil {
		t.Fatal(err)
	}
	// Concurrent senders must not claim the same notification.
	claimed := make(chan domain.Delivery, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { d, e := repo.ClaimDelivery(ctx); claimed <- d; errs <- e }()
	}
	var delivery domain.Delivery
	wins := 0
	for i := 0; i < 2; i++ {
		d := <-claimed
		if d.ID != "" {
			delivery = d
			wins++
		}
		e := <-errs
		if e != nil && e != sql.ErrNoRows {
			t.Fatal(e)
		}
	}
	if wins != 1 || delivery.UserID != uid {
		t.Fatalf("duplicate claim: %d", wins)
	}
	if err = repo.FinishDelivery(ctx, delivery.ID, other, "sent", "wrong", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	var status string
	db.QueryRow(`SELECT status FROM notifications WHERE id=$1`, delivery.ID).Scan(&status)
	if status != "sending" {
		t.Fatal("cross-user update")
	}
	// A 429 schedules a later attempt, which cannot be claimed early.
	if err = repo.FinishDelivery(ctx, delivery.ID, uid, "pending", "", "rate limited", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ClaimDelivery(ctx); err != sql.ErrNoRows {
		t.Fatalf("early retry: %v", err)
	}
	exec(`UPDATE notifications SET scheduled_at=now() WHERE id=$1`, delivery.ID)
	if _, err = repo.ClaimDelivery(ctx); err != nil {
		t.Fatal(err)
	}
	if err = repo.FinishDelivery(ctx, delivery.ID, uid, "sent", "receipt", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ClaimDelivery(ctx); err != sql.ErrNoRows {
		t.Fatal("sent reclaimed")
	}
	history, err := repo.NotificationHistory(ctx, other)
	if err != nil || len(history) != 0 {
		t.Fatal("cross-user history")
	}
	// Delayed result, disabled setting, and changed target must not send.
	for _, mode := range []string{"expired", "disabled", "changed"} {
		exec(`UPDATE user_settings SET notifications_enabled=true,target_bedtime='23:00' WHERE user_id=$1`, uid)
		at := time.Now()
		if mode == "expired" {
			at = at.Add(-time.Hour)
		}
		var id string
		err = db.QueryRow(`INSERT INTO notifications(user_id,decision_job_id,dedup_key,kind,body,scheduled_at,expires_at) VALUES($1,$2,$3,'bedtime','test',$4,$4::timestamptz+interval '15 minutes') RETURNING id`, uid, jobID, mode, at).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		if mode == "disabled" {
			exec(`UPDATE user_settings SET notifications_enabled=false WHERE user_id=$1`, uid)
		}
		if mode == "changed" {
			exec(`UPDATE user_settings SET target_bedtime='22:00' WHERE user_id=$1`, uid)
		}
		if _, err = repo.ClaimDelivery(ctx); err != sql.ErrNoRows {
			t.Fatalf("ineligible claimed %s %v", mode, err)
		}
		if err = repo.RecoverDeliveries(ctx); err != nil {
			t.Fatal(err)
		}
		db.QueryRow(`SELECT status FROM notifications WHERE id=$1`, id).Scan(&status)
		if status != "cancelled" {
			t.Fatalf("not cancelled: %s", mode)
		}
	}
	// Ambiguous delivery after restart is failed, never automatically re-sent.
	exec(`INSERT INTO notifications(user_id,dedup_key,kind,body,scheduled_at,expires_at,status,sending_started_at) VALUES($1,'crash','feedback','test',now(),now()+interval '15 minutes','sending',now()-interval '3 minutes')`, uid)
	if err = repo.RecoverDeliveries(ctx); err != nil {
		t.Fatal(err)
	}
	db.QueryRow(`SELECT status FROM notifications WHERE dedup_key='crash'`).Scan(&status)
	if status != "failed" {
		t.Fatal("ambiguous delivery retried")
	}
	// Explicit tests are idempotent and rate limited, even while automatic sending is off.
	id := platform.UUID()
	if err = repo.CreateTestNotification(ctx, other, id); err != nil {
		t.Fatal(err)
	}
	if err = repo.CreateTestNotification(ctx, other, id); err != nil {
		t.Fatal("idempotent test failed")
	}
	if err = repo.CreateTestNotification(ctx, other, platform.UUID()); err == nil {
		t.Fatal("rate limit missing")
	}
}
