package service

import (
	"context"
	"encoding/json"
	"fmt"
	"suyasuya/internal/domain"
	"time"
)

// DueReminders also checks yesterday so a near-midnight reminder can catch up.
// A daily dedup key suppresses repeated DST wall-clock times and restarts.
func DueReminders(s domain.ReminderSchedule, now time.Time) []domain.ReminderInput {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil || (s.SnoozedUntil != nil && now.Before(*s.SnoozedUntil)) {
		return nil
	}
	local := now.In(loc)
	var due []domain.ReminderInput
	for kind, target := range map[string]string{"bedtime": s.Bedtime, "pvt": s.PVT} {
		if target == "" {
			continue
		}
		for _, day := range []time.Time{local, local.AddDate(0, 0, -1)} {
			at, err := time.ParseInLocation("2006-01-02 15:04", day.Format("2006-01-02")+" "+target, loc)
			if err == nil && at.Format("15:04") == target && !now.Before(at) && now.Sub(at) < 15*time.Minute {
				due = append(due, domain.ReminderInput{Kind: kind, Timezone: s.Timezone, Target: target, DueAt: at})
			}
		}
	}
	return due
}
func (a *Control) scheduleReminders(ctx context.Context) error {
	settings, err := a.repo.ReminderSchedules(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, setting := range settings {
		for _, input := range DueReminders(setting, now) {
			data, _ := json.Marshal(input)
			dedup := fmt.Sprintf("reminder:%s:%s", input.Kind, input.DueAt.Format("2006-01-02"))
			if err = a.repo.ScheduleReminder(ctx, setting.UserID, dedup, a.model, string(data)); err != nil {
				return err
			}
		}
	}
	return nil
}
