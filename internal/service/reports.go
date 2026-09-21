package service

import (
	"context"
	"encoding/json"
	"fmt"
	"suyasuya/internal/domain"
	"suyasuya/internal/platform"
	"time"
	"unicode/utf8"
)

func (a *Control) Analytics(ctx context.Context, u domain.User, start, end string) (any, error) {
	from, err := time.Parse("2006-01-02", start)
	if err != nil {
		return nil, domain.Invalid("開始日を確認してください")
	}
	to, err := time.Parse("2006-01-02", end)
	if err != nil || to.Before(from) || to.Sub(from) > 89*24*time.Hour {
		return nil, domain.Invalid("期間は90日以内で指定してください")
	}
	tz, err := a.repo.UserTimezone(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, err
	}
	rows, err := a.repo.SleepSamples(ctx, u.ID, start, end)
	if err != nil {
		return nil, err
	}
	return domain.SummarizeSleep(start, end, time.Now().In(loc).Format("2006-01-02"), tz, rows), nil
}
func WeekStart(day time.Time) time.Time {
	offset := (int(day.Weekday()) + 6) % 7
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location()).AddDate(0, 0, -offset)
}
func DueWeekly(s domain.WeeklySchedule, now time.Time) (time.Time, bool) {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil || (s.SnoozedUntil != nil && now.Before(*s.SnoozedUntil)) {
		return time.Time{}, false
	}
	local := now.In(loc)
	for _, day := range []time.Time{local, local.AddDate(0, 0, -1)} {
		if int(day.Weekday()) != s.Weekday {
			continue
		}
		due, err := time.ParseInLocation("2006-01-02 15:04", day.Format("2006-01-02")+" "+s.Target, loc)
		if err == nil && due.Format("15:04") == s.Target && !now.Before(due) && now.Sub(due) < 24*time.Hour {
			return due, true
		}
	}
	return time.Time{}, false
}
func (a *Control) Reports(ctx context.Context, u domain.User) (any, error) {
	return a.repo.Reports(ctx, u.ID)
}

type ReportRequest struct {
	WeekStart    string `json:"week_start"`
	SubmissionID string `json:"submission_id"`
	Deliver      bool   `json:"deliver"`
}

func (a *Control) GenerateReport(ctx context.Context, u domain.User, in ReportRequest) (any, error) {
	if !platform.ValidID(in.SubmissionID) {
		return nil, domain.Invalid("生成IDを確認してください")
	}
	tz, err := a.repo.UserTimezone(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, err
	}
	day, err := time.ParseInLocation("2006-01-02", in.WeekStart, loc)
	if err != nil || day.After(time.Now()) || day.Before(time.Now().AddDate(-10, 0, 0)) {
		return nil, domain.Invalid("対象週の日付を確認してください")
	}
	start := WeekStart(day)
	input := domain.WeeklyInput{Deliver: in.Deliver, Timezone: tz, DueAt: time.Now(), Report: domain.SleepAnalytics{Start: start.Format("2006-01-02"), End: start.AddDate(0, 0, 6).Format("2006-01-02")}}
	id, err := a.createWeekly(ctx, u.ID, "manual-weekly:"+in.SubmissionID, input)
	if err != nil {
		return nil, err
	}
	a.enqueue(ctx, id)
	return map[string]string{"id": id}, nil
}
func (a *Control) createWeekly(ctx context.Context, userID, dedup string, in domain.WeeklyInput) (string, error) {
	tx, err := a.repo.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if err = tx.LockNotificationSettings(ctx, userID); err != nil {
		return "", err
	}
	if in.Deliver {
		configured, err := tx.WebhookConfigured(ctx, userID)
		if err != nil {
			return "", err
		}
		if !configured {
			return "", domain.Invalid("Webhook を設定してください")
		}
	}
	exists, err := tx.WeeklyJobExists(ctx, userID, dedup)
	if err != nil {
		return "", err
	}
	if !exists && !in.Automatic {
		var count int
		if err = tx.RecentWeeklyCount(ctx, userID, &count); err != nil {
			return "", err
		}
		if count >= 5 {
			return "", domain.Invalid("少し待ってから生成してください")
		}
	}
	rows, err := tx.SleepSamples(ctx, userID, in.Report.Start, in.Report.End)
	if err != nil {
		return "", err
	}
	loc, err := time.LoadLocation(in.Timezone)
	if err != nil {
		return "", err
	}
	in.Report = domain.SummarizeSleep(in.Report.Start, in.Report.End, time.Now().In(loc).Format("2006-01-02"), in.Timezone, rows)
	data, err := json.Marshal(in)
	if err != nil {
		return "", err
	}
	id, err := tx.CreateWeeklyJob(ctx, userID, dedup, a.model, string(data))
	if err != nil {
		return "", err
	}
	if err = tx.BindWeeklyRecords(ctx, id, userID); err != nil {
		return "", err
	}
	return id, tx.Commit()
}
func (a *Control) scheduleWeekly(ctx context.Context) error {
	schedules, err := a.repo.WeeklySchedules(ctx)
	if err != nil {
		return err
	}
	for _, s := range schedules {
		due, ok := DueWeekly(s, time.Now())
		if !ok {
			continue
		}
		start := WeekStart(due).AddDate(0, 0, -7)
		dedup := "weekly:" + start.Format("2006-01-02")
		exists, err := a.repo.WeeklyJobExists(ctx, s.UserID, dedup)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		input := domain.WeeklyInput{Automatic: true, Deliver: true, Timezone: s.Timezone, Target: s.Target, Weekday: s.Weekday, DueAt: due, Report: domain.SleepAnalytics{Start: start.Format("2006-01-02"), End: start.AddDate(0, 0, 6).Format("2006-01-02")}}
		if _, err = a.createWeekly(ctx, s.UserID, dedup, input); err != nil {
			return err
		}
	}
	return nil
}
func (a *Control) collectWeekly(ctx context.Context) error {
	plans, err := a.repo.UnsentWeeklyReports(ctx)
	if err != nil {
		return err
	}
	for _, p := range plans {
		var in domain.WeeklyInput
		var out domain.WeeklyResult
		if json.Unmarshal(p.Input, &in) != nil || json.Unmarshal(p.Output, &out) != nil {
			return fmt.Errorf("invalid weekly report result")
		}
		if out.Send != nil && !*out.Send {
			if err = a.repo.SuppressNotification(ctx, p, "weekly_report", in.DueAt, out.Body, out.Reason); err != nil {
				return err
			}
			continue
		}
		if out.Body == "" || utf8.RuneCountInString(out.Body) > 1500 {
			return fmt.Errorf("invalid agent report")
		}
		if err = a.repo.CreateWeeklyNotification(ctx, p, out.Body, in.DueAt); err != nil {
			return err
		}
	}
	return nil
}
