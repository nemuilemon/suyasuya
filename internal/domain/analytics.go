package domain

import (
	"fmt"
	"math"
	"time"
)

type SleepSample struct {
	ID        string     `json:"id"`
	Revision  int        `json:"revision"`
	Date      string     `json:"date"`
	Kind      string     `json:"kind"`
	Minutes   *int       `json:"minutes"`
	StartedAt *time.Time `json:"started_at"`
}
type SleepDay struct {
	Date           string `json:"date"`
	MainMinutes    *int   `json:"main_minutes"`
	NapMinutes     *int   `json:"nap_minutes"`
	MainIncomplete bool   `json:"main_incomplete"`
	NapIncomplete  bool   `json:"nap_incomplete"`
	Bedtime        *int   `json:"bedtime"`
}
type SleepAnalytics struct {
	Start          string        `json:"start"`
	End            string        `json:"end"`
	Through        string        `json:"through"`
	Timezone       string        `json:"timezone"`
	Days           []SleepDay    `json:"days"`
	MainDays       int           `json:"main_days"`
	DurationDays   int           `json:"duration_days"`
	AverageMain    *int          `json:"average_main"`
	AverageBedtime *int          `json:"average_bedtime"`
	NapCount       int           `json:"nap_count"`
	NapMinutes     int           `json:"nap_minutes"`
	Samples        []SleepSample `json:"samples"`
}

func ClockMinutes(t time.Time) int {
	n := t.Hour()*60 + t.Minute()
	if n < 720 {
		n += 1440
	}
	return n
}
func ClockLabel(n int) string { n %= 1440; return fmt.Sprintf("%02d:%02d", n/60, n%60) }
func SummarizeSleep(start, end, through, tz string, records []SleepSample) SleepAnalytics {
	out := SleepAnalytics{Start: start, End: end, Through: through, Timezone: tz, Days: []SleepDay{}, Samples: []SleepSample{}}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return out
	}
	from, _ := time.Parse("2006-01-02", start)
	to, _ := time.Parse("2006-01-02", end)
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		out.Days = append(out.Days, SleepDay{Date: d.Format("2006-01-02")})
	}
	byDate := map[string][]SleepSample{}
	for _, r := range records {
		if r.Date >= start && r.Date <= end && r.Date <= through {
			byDate[r.Date] = append(byDate[r.Date], r)
			out.Samples = append(out.Samples, r)
		}
	}
	total, bedTotal, bedCount := 0, 0, 0
	for i := range out.Days {
		day := &out.Days[i]
		hasMain := false
		var earliest *time.Time
		for _, r := range byDate[day.Date] {
			if r.Kind == "main" {
				hasMain = true
				if r.Minutes == nil {
					day.MainIncomplete = true
				} else {
					if day.MainMinutes == nil {
						day.MainMinutes = new(int)
					}
					*day.MainMinutes += *r.Minutes
				}
				if r.StartedAt != nil && (earliest == nil || r.StartedAt.Before(*earliest)) {
					earliest = r.StartedAt
				}
			} else if r.Kind == "nap" {
				out.NapCount++
				if r.Minutes == nil {
					day.NapIncomplete = true
				} else {
					if day.NapMinutes == nil {
						day.NapMinutes = new(int)
					}
					*day.NapMinutes += *r.Minutes
					out.NapMinutes += *r.Minutes
				}
			}
		}
		if hasMain {
			out.MainDays++
		}
		if day.MainMinutes != nil && !day.MainIncomplete {
			out.DurationDays++
			total += *day.MainMinutes
		}
		if earliest != nil {
			v := ClockMinutes(earliest.In(loc))
			day.Bedtime = &v
			bedTotal += v
			bedCount++
		}
	}
	if out.DurationDays > 0 {
		n := int(math.Round(float64(total) / float64(out.DurationDays)))
		out.AverageMain = &n
	}
	if bedCount > 0 {
		n := int(math.Round(float64(bedTotal) / float64(bedCount)))
		out.AverageBedtime = &n
	}
	return out
}

type WeeklyInput struct {
	Report    SleepAnalytics `json:"report"`
	Deliver   bool           `json:"deliver"`
	Automatic bool           `json:"automatic"`
	Timezone  string         `json:"timezone"`
	Target    string         `json:"target"`
	Weekday   int            `json:"weekday"`
	DueAt     time.Time      `json:"due_at"`
}
type WeeklyResult struct {
	Send     *bool  `json:"send,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Body     string `json:"body"`
	Fallback bool   `json:"fallback"`
}
type WeeklySchedule struct {
	UserID, Timezone, Target string
	Weekday                  int
	SnoozedUntil             *time.Time
}

const WeeklyPrompt = `対象週の睡眠を振り返り、ユーザーが自分の睡眠の傾向を理解できるレポートを作る。承認済み記録のsnapshotはgetSleepTimeから取得できる。必要なtoolを使い、何を伝えるかと送信の要否を判断する。`
