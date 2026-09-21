package repository

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"suyasuya/internal/platform"
	"testing"
	"time"
)

func TestPersonalAgentsIntegration(t *testing.T) {
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
	u, other := platform.UUID(), platform.UUID()
	defer cleanupNotificationFixtures(t, db, u, other)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO users(id,username,password_hash) VALUES($1::uuid,$1::text,'test'),($2::uuid,$2::text,'test')`, u, other)
	exec(`INSERT INTO user_settings(user_id) VALUES($1),($2)`, u, other)
	for id, body := range map[string]string{u: "persona one", other: "persona two"} {
		if _, err = repo.EnsurePrompt(ctx, "agent.loop", body, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = repo.RetirePrompt(ctx, "agent.loop", u); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.InsertPrompt(ctx, "agent.loop", "persona one revised", u); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{u: "persona one revised", other: "persona two"} {
		job := platform.UUID()
		exec(`INSERT INTO jobs(id,user_id,kind,dedup_key,model) VALUES($1::uuid,$2,'future_task',$1::text,'test')`, job, id)
		body, err := repo.TaskBasePrompt(ctx, job, id)
		if err != nil || body != want {
			t.Fatalf("wrong persona %q %v", body, err)
		}
		history, err := repo.Prompts(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		expected := 1
		if id == u {
			expected = 2
		}
		if len(history) != expected {
			t.Fatal("prompt history crosses users")
		}
	}
	// A skipped notification remains in history and can never be claimed.
	job := platform.UUID()
	exec(`INSERT INTO jobs(id,user_id,kind,dedup_key,model) VALUES($1::uuid,$2,'plan_reminder',$1::text,'test')`, job, u)
	exec(`INSERT INTO notification_destinations(user_id,discord_webhook_url) VALUES($1,'https://discord.com/api/webhooks/1/test')`, u)
	if err = repo.SuppressNotification(ctx, PlannedReminder{ID: job, UserID: u, Dedup: "skip"}, "bedtime", time.Now(), "", "already notified"); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = db.QueryRow(`SELECT status FROM notifications WHERE user_id=$1`, u).Scan(&status); err != nil || status != "cancelled" {
		t.Fatal("skip not saved")
	}
	if _, err = repo.ClaimDelivery(ctx); err != sql.ErrNoRows {
		t.Fatal("skipped delivery claimed")
	}
}
