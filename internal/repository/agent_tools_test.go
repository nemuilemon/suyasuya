package repository

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"suyasuya/internal/platform"
	"testing"
)

func TestAgentToolQueriesIntegration(t *testing.T) {
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
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	repo := New(db)
	u, other := platform.UUID(), platform.UUID()
	defer cleanupNotificationFixtures(t, db, u, other)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO users(id,username,password_hash) VALUES($1::uuid,$1::text,'test'),($2::uuid,$2::text,'test')`, u, other)
	exec(`INSERT INTO user_settings(user_id) VALUES($1),($2)`, u, other)
	exec(`INSERT INTO notification_destinations(user_id,discord_webhook_url) VALUES($1,'never-show-secret')`, u)
	exec(`INSERT INTO sleep_records(user_id,status,sleep_kind,sleep_date,timezone,duration_minutes,source,submission_id) VALUES($1,'approved','main','2026-09-20','Asia/Tokyo',420,'manual',gen_random_uuid()),($1,'rejected','nap','2026-09-20','Asia/Tokyo',50,'manual',gen_random_uuid()),($2,'approved','main','2026-09-20','Asia/Tokyo',999,'manual',gen_random_uuid())`, u, other)
	exec(`INSERT INTO pvt_sessions(user_id,started_at,finished_at,timezone,device,input_method,trials,median_rt_ms) VALUES($1,'2026-09-19 15:05Z','2026-09-19 15:06Z','Asia/Tokyo','test','test','[]',234),($2,'2026-09-19 15:05Z','2026-09-19 15:06Z','Asia/Tokyo','test','test','[]',999)`, u, other)
	defer exec(`DELETE FROM pvt_sessions WHERE user_id IN ($1,$2)`, u, other)
	exec(`SET ROLE suyasuya_agent`)
	defer exec(`RESET ROLE`)
	rows, err := repo.AgentSleepSamples(ctx, u, "2026-09-20", "2026-09-20")
	if err != nil || len(rows) != 1 || *rows[0].Minutes != 420 {
		t.Fatal("sleep tools access/owner filter", err)
	}
	sessions, err := repo.AgentPVT(ctx, u, "2026-09-20", "2026-09-20", "Asia/Tokyo")
	if err != nil || len(sessions) != 1 || !strings.Contains(string(sessions[0]), "234") {
		t.Fatal("PVT timezone/owner filter", err)
	}
	settings, err := repo.AgentSettings(ctx, u)
	if err != nil || strings.Contains(string(settings), "webhook") || strings.Contains(string(settings), "secret") {
		t.Fatal("settings leaked secrets or unavailable", err)
	}
}
