package service

import (
	"suyasuya/internal/domain"
	"testing"
	"time"
)

func TestWeeklySchedule(t *testing.T) {
	s := domain.WeeklySchedule{Timezone: "Asia/Tokyo", Weekday: 1, Target: "09:00"}
	for _, tc := range []struct {
		now  string
		want bool
	}{
		{"2026-09-21T08:59:59+09:00", false}, {"2026-09-21T09:00:00+09:00", true}, {"2026-09-22T08:59:59+09:00", true}, {"2026-09-22T09:00:00+09:00", false}, {"2026-09-23T09:00:00+09:00", false},
	} {
		now, _ := time.Parse(time.RFC3339, tc.now)
		due, ok := DueWeekly(s, now)
		if ok != tc.want {
			t.Errorf("%s got %v", tc.now, ok)
		}
		if ok && WeekStart(due).AddDate(0, 0, -7).Format("2006-01-02") != "2026-09-14" {
			t.Fatal("wrong reporting week")
		}
	}
	date, _ := time.Parse(time.RFC3339, "2026-01-01T12:00:00+09:00")
	if WeekStart(date).Format("2006-01-02") != "2025-12-29" {
		t.Fatal("year boundary")
	}
	snooze := date.AddDate(1, 0, 0)
	s.SnoozedUntil = &snooze
	if _, ok := DueWeekly(s, date); ok {
		t.Fatal("snoozed schedule")
	}
}
