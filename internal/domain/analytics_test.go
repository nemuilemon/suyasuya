package domain

import (
	"testing"
	"time"
)

func TestSleepSummaryMissingAndMidnight(t *testing.T) {
	minutes := func(n int) *int { return &n }
	at := func(s string) *time.Time { v, _ := time.Parse(time.RFC3339, s); return &v }
	rows := []SleepSample{
		{Date: "2026-09-14", Kind: "main", Minutes: minutes(420), StartedAt: at("2026-09-13T23:00:00+09:00")},
		{Date: "2026-09-15", Kind: "main", Minutes: minutes(480), StartedAt: at("2026-09-15T01:00:00+09:00")},
		{Date: "2026-09-15", Kind: "nap", Minutes: minutes(30)},
		{Date: "2026-09-16", Kind: "main", Minutes: nil},
		{Date: "2026-09-16", Kind: "nap", Minutes: nil},
		{Date: "2026-09-17", Kind: "main", Minutes: minutes(10)},
		{Date: "2026-09-17", Kind: "main", Minutes: nil},
		{Date: "2026-09-20", Kind: "main", Minutes: minutes(999)},
	}
	a := SummarizeSleep("2026-09-14", "2026-09-20", "2026-09-19", "Asia/Tokyo", rows)
	if a.MainDays != 4 || a.DurationDays != 2 || a.AverageMain == nil || *a.AverageMain != 450 {
		t.Fatalf("unknown durations must not count as zero: %+v", a)
	}
	if a.AverageBedtime == nil || ClockLabel(*a.AverageBedtime) != "00:00" {
		t.Fatal("midnight average incorrect")
	}
	if a.NapCount != 2 || a.NapMinutes != 30 {
		t.Fatal("nap totals")
	}
	if len(a.Days) != 7 || a.Days[4].MainMinutes != nil || !a.Days[3].MainIncomplete {
		t.Fatal("missing days or partial durations")
	}
	if len(a.Samples) != 7 {
		t.Fatal("future records must be excluded")
	}
}
