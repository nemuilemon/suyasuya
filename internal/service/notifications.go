package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"suyasuya/internal/domain"
	"suyasuya/internal/platform"
	"time"
	"unicode/utf8"
)

func (a *Control) Notifications(ctx context.Context, u domain.User) (any, error) {
	return a.repo.NotificationHistory(ctx, u.ID)
}
func (a *Control) TestNotification(ctx context.Context, u domain.User, id string) (any, error) {
	if !platform.ValidID(id) {
		return nil, domain.Invalid("送信IDを確認してください")
	}
	tx, err := a.repo.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = tx.LockNotificationSettings(ctx, u.ID); err != nil {
		return nil, err
	}
	if err = tx.CreateTestNotification(ctx, u.ID, id); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]bool{"queued": true}, nil
}
func (a *Control) NotificationsLoop(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for ctx.Err() == nil {
		cycle, cancel := context.WithTimeout(ctx, time.Minute)
		if err := a.processNotifications(cycle); err != nil && ctx.Err() == nil {
			log.Printf("notifications: %s", platform.SafeError(err))
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (a *Control) processNotifications(ctx context.Context) error {
	if err := a.scheduleReminders(ctx); err != nil {
		return err
	}
	if err := a.scheduleWeekly(ctx); err != nil {
		return err
	}
	if err := a.collectWeekly(ctx); err != nil {
		return err
	}
	plans, err := a.repo.PlannedReminders(ctx)
	if err != nil {
		return err
	}
	for _, p := range plans {
		var in domain.ReminderInput
		var out domain.ReminderResult
		if json.Unmarshal(p.Input, &in) != nil || json.Unmarshal(p.Output, &out) != nil || (in.Kind != "bedtime" && in.Kind != "pvt") {
			continue
		}
		if out.Send != nil && !*out.Send {
			if err = a.repo.SuppressNotification(ctx, p, in.Kind, in.DueAt, out.Body, out.Reason); err != nil {
				return err
			}
			continue
		}
		if strings.TrimSpace(out.Body) == "" || utf8.RuneCountInString(out.Body) > 1500 {
			return fmt.Errorf("invalid agent message")
		}
		if err = a.repo.CreateReminderNotification(ctx, p, in, out.Body); err != nil {
			return err
		}
	}
	if err = a.repo.RecoverDeliveries(ctx); err != nil {
		return err
	}
	for i := 0; i < 10 && ctx.Err() == nil; i++ {
		delivery, err := a.repo.ClaimDelivery(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		result := a.notifier.Send(ctx, delivery.Webhook, delivery.Body, delivery.AgentName)
		status := "failed"
		if result.MessageID != "" {
			status = "sent"
		} else if result.RetryAfter > 0 && delivery.Attempts < 3 {
			status = "pending"
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = a.repo.FinishDelivery(cleanup, delivery.ID, delivery.UserID, status, result.MessageID, result.Error, time.Now().Add(result.RetryAfter))
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}
