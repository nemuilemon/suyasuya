package service

import (
	"suyasuya/internal/domain"
	"testing"
	"time"
)

func TestDueReminders(t *testing.T) {
	for _, tc := range []struct {
		name, now, bedtime, pvt string
		count                   int
	}{
		{"before", "2026-09-20T22:59:59+09:00", "23:00", "", 0},
		{"on time", "2026-09-20T23:00:00+09:00", "23:00", "", 1},
		{"catchup", "2026-09-20T23:14:59+09:00", "23:00", "", 1},
		{"expired", "2026-09-20T23:15:00+09:00", "23:00", "", 0},
		{"midnight", "2026-09-21T00:04:00+09:00", "23:55", "", 1},
		{"both", "2026-09-20T23:00:00+09:00", "23:00", "23:00", 2},
		{"unset", "2026-09-20T23:00:00+09:00", "", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now, _ := time.Parse(time.RFC3339, tc.now)
			s := domain.ReminderSchedule{Timezone: "Asia/Tokyo", Bedtime: tc.bedtime, PVT: tc.pvt}
			out := DueReminders(s, now)
			if len(out) != tc.count {
				t.Fatalf("got %d", len(out))
			}
			if tc.name == "midnight" && out[0].DueAt.Format("2006-01-02") != "2026-09-20" {
				t.Fatal("wrong local date")
			}
			later := now.Add(time.Hour)
			s.SnoozedUntil = &later
			if len(DueReminders(s, now)) != 0 {
				t.Fatal("snoozed")
			}
		})
	}
}

func TestNonexistentDSTTimeIsSkipped(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-03-08T01:30:00-05:00")
	setting := domain.ReminderSchedule{Timezone: "America/New_York", Bedtime: "02:30"}
	if len(DueReminders(setting, now)) != 0 {
		t.Fatal("nonexistent wall clock time sent early")
	}
}
