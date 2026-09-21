package repository

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"suyasuya/internal/platform"
	"testing"
)

func TestTaskBasePromptIntegration(t *testing.T) {
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
	uid, other := platform.UUID(), platform.UUID()
	defer cleanupNotificationFixtures(t, db, uid, other)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO users(id,username,password_hash) VALUES($1::uuid,$1::text,'test'),($2::uuid,$2::text,'test')`, uid, other)
	exec(`INSERT INTO agent_config_versions(config_key,kind,version,body,is_active,created_by) VALUES('agent.loop','prompt',1,'base one',true,$1)`, uid)
	exec(`SET ROLE suyasuya_control`)
	defer exec(`RESET ROLE`)
	for _, kind := range []string{"plan_reminder", "weekly_report", "future_task"} {
		id := platform.UUID()
		exec(`INSERT INTO jobs(id,user_id,kind,dedup_key,model) VALUES($1::uuid,$2,$3,$1::text,'test')`, id, uid, kind)
		base, err := repo.TaskBasePrompt(ctx, id, uid)
		if err != nil || base != "base one" {
			t.Fatalf("%s base: %q %v", kind, base, err)
		}
		if _, err = repo.TaskBasePrompt(ctx, id, other); err == nil {
			t.Fatal("cross-user prompt read")
		}
	}
	upload, id := platform.UUID(), platform.UUID()
	exec(`INSERT INTO uploads(id,user_id,content_type,bucket,object_name,expected_size_bytes,expires_at) VALUES($1,$2,'image/png','test','test',1,now()+interval '15 minutes')`, upload, uid)
	defer exec(`DELETE FROM uploads WHERE id=$1`, upload)
	exec(`INSERT INTO jobs(id,user_id,kind,dedup_key,model,upload_id) VALUES($1::uuid,$2,'extract_sleep',$1::text,'test',$3)`, id, uid, upload)
	defer exec(`DELETE FROM job_config_versions WHERE job_id=$1`, id)
	// Explicit cleanup order for upload FK is handled below.
	exec(`UPDATE agent_config_versions SET is_active=false WHERE config_key='agent.loop'`)
	exec(`INSERT INTO agent_config_versions(config_key,kind,version,body,is_active,created_by) VALUES('agent.loop','prompt',2,'base two',true,$1)`, uid)
	base, err := repo.TaskBasePrompt(ctx, id, uid)
	if err != nil || base != "base one" {
		t.Fatal("existing job changed with base edit")
	}
	newID := platform.UUID()
	exec(`INSERT INTO jobs(id,user_id,kind,dedup_key,model) VALUES($1::uuid,$2,'future_task',$1::text,'test')`, newID, uid)
	base, err = repo.TaskBasePrompt(ctx, newID, uid)
	if err != nil || base != "base two" {
		t.Fatal("new job did not receive new base")
	}
	exec(`DELETE FROM job_config_versions WHERE job_id=$1`, id)
	exec(`DELETE FROM jobs WHERE id=$1`, id)
}
