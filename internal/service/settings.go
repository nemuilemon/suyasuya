package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"suyasuya/internal/notifier"

	"suyasuya/internal/domain"
)

func (a *Control) Settings(ctx context.Context, u domain.User) (any, error) {
	var b []byte
	e := a.repo.Settings(ctx, u.ID, &b)
	if e != nil {
		return nil, e
	}
	return json.RawMessage(b), nil
}

func (a *Control) SaveSettings(ctx context.Context, u domain.User, in domain.SettingsInput) (any, error) {
	if _, e := time.LoadLocation(in.Timezone); e != nil {
		return nil, domain.Invalid("タイムゾーンを確認してください")
	}
	for _, s := range []string{in.PVT, in.Bedtime} {
		if s != "" {
			if _, e := time.Parse("15:04", s); e != nil {
				return nil, domain.Invalid("時刻を確認してください")
			}
		}
	}
	if in.AgentName != nil {
		*in.AgentName = strings.TrimSpace(*in.AgentName)
		if utf8.RuneCountInString(*in.AgentName) > 80 || strings.ContainsAny(*in.AgentName, "\r\n") {
			return nil, domain.Invalid("表示名は80文字以内で入力してください")
		}
	}
	if in.WeeklyWeekday != nil && (*in.WeeklyWeekday < 0 || *in.WeeklyWeekday > 6) {
		return nil, domain.Invalid("週次送信の曜日を確認してください")
	}
	if in.WeeklyTime != nil {
		if _, err := time.Parse("15:04", *in.WeeklyTime); err != nil {
			return nil, domain.Invalid("週次送信の時刻を確認してください")
		}
	}
	if in.Webhook != nil {
		*in.Webhook = strings.TrimSpace(*in.Webhook)
	}
	if in.Webhook != nil && *in.Webhook != "" {
		if in.ClearWebhook || !validWebhook(*in.Webhook) {
			return nil, domain.Invalid("Discord Webhook URL を確認してください")
		}
	}
	tx, e := a.repo.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	if e = tx.LockNotificationSettings(ctx, u.ID); e != nil {
		return nil, e
	}
	if in.AgentName != nil {
		if e = tx.SetAgentName(ctx, u.ID, *in.AgentName); e != nil {
			return nil, e
		}
	}
	if in.ClearWebhook {
		e = tx.SetWebhook(ctx, u.ID, "")
	} else if in.Webhook != nil && *in.Webhook != "" {
		e = tx.SetWebhook(ctx, u.ID, *in.Webhook)
	}
	if e != nil {
		return nil, e
	}
	_, e = tx.SaveSettings(ctx, u.ID, in.Timezone, in.PVT, in.Bedtime)
	if e != nil {
		return nil, e
	}
	if in.ClearWebhook {
		off := false
		in.NotificationsEnabled = &off
	}
	if e = tx.SaveWeeklySettings(ctx, u.ID, in.WeeklyEnabled, in.WeeklyWeekday, in.WeeklyTime); e != nil {
		return nil, e
	}
	if in.NotificationsEnabled != nil {
		if *in.NotificationsEnabled {
			configured, err := tx.WebhookConfigured(ctx, u.ID)
			if err != nil {
				return nil, err
			}
			weekly, err := tx.WeeklyEnabled(ctx, u.ID)
			if err != nil {
				return nil, err
			}
			if !configured || (in.Bedtime == "" && in.PVT == "" && !weekly) {
				return nil, domain.Invalid("Webhook と通知する時刻を設定してください")
			}
		}
		if e = tx.SetNotificationsEnabled(ctx, u.ID, *in.NotificationsEnabled); e != nil {
			return nil, e
		}
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return a.Settings(ctx, u)
}

func (a *Control) Prompts(ctx context.Context, u domain.User) (any, error) {
	if err := a.EnsurePersonalAgent(ctx, u.ID); err != nil {
		return nil, err
	}
	out, e := a.repo.Prompts(ctx, u.ID)
	if e != nil {
		return nil, e
	}
	return out, nil
}

func (a *Control) SavePrompt(ctx context.Context, u domain.User, in domain.PromptInput) (any, error) {
	if in.Key != "job.extract_sleep" && in.Key != "agent.loop" && in.Key != "job.plan_reminder" && in.Key != "job.weekly_report" {
		return nil, domain.Invalid("未対応のプロンプトです")
	}
	if len(strings.TrimSpace(in.Body)) == 0 || len(in.Body) > 32000 {
		return nil, domain.Invalid("本文は1〜32000 bytesで指定してください")
	}
	tx, e := a.repo.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	_, e = tx.LockPrompt(ctx, in.Key, u.ID)
	if e != nil {
		return nil, e
	}
	_, e = tx.RetirePrompt(ctx, in.Key, u.ID)
	if e != nil {
		return nil, e
	}
	_, e = tx.InsertPrompt(ctx, in.Key, in.Body, u.ID)
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return a.Prompts(ctx, u)
}

func validWebhook(raw string) bool { return notifier.ValidURL(raw) }
