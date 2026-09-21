package agent

import (
	"context"
	"encoding/json"
	"suyasuya/internal/domain"
	"testing"
	"time"
)

type fakeData struct {
	seen       []string
	sleepReads int
}

func (f *fakeData) UserTimezone(_ context.Context, id string) (string, error) {
	f.seen = append(f.seen, id)
	return "Asia/Tokyo", nil
}
func (f *fakeData) AgentSleepSamples(_ context.Context, id, from, to string) ([]domain.SleepSample, error) {
	f.seen = append(f.seen, id)
	f.sleepReads++
	n := 420
	return []domain.SleepSample{{Date: from, Kind: "main", Minutes: &n}}, nil
}
func (f *fakeData) AgentPVT(_ context.Context, id, from, to, tz string) ([]json.RawMessage, error) {
	f.seen = append(f.seen, id)
	return []json.RawMessage{json.RawMessage(`{"completed":true,"protocol_version":"pvt-1m-v1"}`)}, nil
}
func (f *fakeData) AgentSettings(_ context.Context, id string) (json.RawMessage, error) {
	f.seen = append(f.seen, id)
	return json.RawMessage(`{"timezone":"Asia/Tokyo"}`), nil
}
func TestToolsOwnerAndValidation(t *testing.T) {
	f := &fakeData{}
	r := NewToolRegistry(f, domain.Task{UserID: "owner"})
	r.now = time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()
	for _, name := range []string{"getSleepTime", "getPVTResults", "getUserSettings"} {
		if out := r.Call(ctx, name, nil); out["error"] != nil {
			t.Fatal(out)
		}
	}
	for _, id := range f.seen {
		if id != "owner" {
			t.Fatal("owner was not bound")
		}
	}
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"getSleepTime", map[string]any{"user_id": "other"}},
		{"getSleepTime", map[string]any{"from": "2026-01-01", "to": "2026-09-01"}},
		{"getSleepTime", map[string]any{"from": 123, "to": "2026-09-01"}},
		{"getSleepTime", map[string]any{"from": "2026-09-20", "to": "2026-09-19"}},
		{"executeSQL", nil},
	} {
		if r.Call(ctx, tc.name, tc.args)["error"] == nil {
			t.Fatal("invalid tool call accepted", tc)
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if r.Call(ctx, "getSleepTime", nil)["error"] != "cancelled" {
		t.Fatal("cancel ignored")
	}
}
func TestWeeklySleepToolUsesSnapshot(t *testing.T) {
	f := &fakeData{}
	minutes := 123
	snapshot := domain.WeeklyInput{Report: domain.SleepAnalytics{Start: "2026-09-14", End: "2026-09-20", Through: "2026-09-20", Timezone: "Asia/Tokyo", Samples: []domain.SleepSample{{Date: "2026-09-14", Kind: "main", Minutes: &minutes}}}}
	data, _ := json.Marshal(snapshot)
	r := NewToolRegistry(f, domain.Task{UserID: "owner", Kind: "weekly_report", Input: data})
	out := r.Call(context.Background(), "getSleepTime", nil)
	if out["error"] != nil {
		t.Fatal(out)
	}
	summary := out["output"].(domain.SleepAnalytics)
	if *summary.AverageMain != 123 || f.sleepReads != 0 {
		t.Fatal("snapshot replaced by live data")
	}
	if r.Call(context.Background(), "getSleepTime", map[string]any{"from": "2026-09-01", "to": "2026-09-07"})["error"] == nil {
		t.Fatal("outside snapshot accepted")
	}
}

func (f *fakeData) AgentNotificationHistory(_ context.Context, id string) ([]json.RawMessage, error) {
	f.seen = append(f.seen, id)
	return []json.RawMessage{}, nil
}
func TestGoalToolAllowlist(t *testing.T) {
	r := NewToolRegistry(&fakeData{}, domain.Task{UserID: "owner"})
	r.allowed = []string{"getSleepTime"}
	if len(r.Declarations()) != 1 {
		t.Fatal("tool declarations not filtered")
	}
	if r.Call(context.Background(), "getUserSettings", nil)["error"] == nil {
		t.Fatal("disallowed tool executed")
	}
}
