package service

import (
	"testing"
	"time"

	"suyasuya/internal/domain"
)

func ptr[T any](v T) *T { return &v }

func TestMeasureRecomputesMetrics(t *testing.T) {
	now := time.Now()
	in := domain.PVTInput{Started: now.Add(-60 * time.Second), Finished: now, Completed: true, Trials: []domain.Trial{{Response: 200}, {Stimulus: ptr(1000.0), Response: 1050}, {Stimulus: ptr(2000.0), Response: 2200, False: true}, {Stimulus: ptr(3000.0), Response: 3355}, {Stimulus: ptr(4000.0), Response: 4500}}}
	m, e := measure(&in)
	if e != nil {
		t.Fatal(e)
	}
	if m.Median == nil || *m.Median != 355 || m.Lapses != 2 || m.False != 2 || *m.Slow != 500 {
		t.Fatalf("wrong domain.Metrics: %+v", m)
	}
	if in.Trials[2].False || *in.Trials[2].RT != 200 {
		t.Fatal("client domain.Metrics were trusted")
	}
}

func TestMeasureRejectsInvalidTimeline(t *testing.T) {
	now := time.Now()
	for _, in := range []domain.PVTInput{{Started: now.Add(-time.Second), Finished: now, Completed: true}, {Started: now.Add(-time.Second), Finished: now, Trials: []domain.Trial{{Response: 500}, {Response: 400}}}, {Started: now.Add(-time.Second), Finished: now, Trials: []domain.Trial{{Stimulus: ptr(600.0), Response: 500}}}} {
		if _, e := measure(&in); e == nil {
			t.Fatal("invalid timeline accepted")
		}
	}
}

func TestRecordOvernightAndNap(t *testing.T) {
	in := domain.RecordInput{Timezone: "Asia/Tokyo", Date: "2026-09-18", Kind: "main", Status: "approved", Start: "2026-09-17T23:30", End: "2026-09-18T07:30", Duration: ptr(450)}
	a, b, e := parseRecord(in)
	if e != nil || b.Sub(*a) != 8*time.Hour {
		t.Fatalf("overnight: %v", e)
	}
	in.Kind = "nap"
	in.Start = ""
	in.End = ""
	in.Duration = ptr(20)
	if _, _, e = parseRecord(in); e != nil {
		t.Fatal(e)
	}
	in.Duration = nil
	if _, _, e = parseRecord(in); e == nil {
		t.Fatal("empty record accepted")
	}
}

func TestOneMinuteProtocol(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		seconds   float64
		completed bool
		valid     bool
	}{{60, true, true}, {59.9, true, false}, {59.9, false, true}, {180, true, false}} {
		in := domain.PVTInput{Started: now.Add(-time.Duration(tc.seconds * float64(time.Second))), Finished: now, Completed: tc.completed}
		_, err := measure(&in)
		if (err == nil) != tc.valid {
			t.Fatalf("duration=%v completed=%v error=%v", tc.seconds, tc.completed, err)
		}
	}
}
