package service

import (
	"context"
	"log"
	"time"

	"suyasuya/internal/platform"
)

func (a *Control) enqueue(ctx context.Context, id string) {
	if ok, e := a.queue.SetNX(ctx, "queued:"+id, "1", 15*time.Second).Result(); e == nil && ok {
		if e = a.queue.LPush(ctx, "ocr:pending", id).Err(); e != nil {
			a.queue.Del(ctx, "queued:"+id)
		}
	}
}
func (a *Control) Sweep(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		a.requeue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (a *Control) requeue(ctx context.Context) {
	_, e := a.repo.RecoverExpiredJobs(ctx)
	if e != nil {
		log.Printf("sweeper: %s", platform.SafeError(e))
		return
	}
	rows, e := a.repo.PendingJobs(ctx)
	if e != nil {
		return
	}
	for _, id := range rows {
		a.enqueue(ctx, id)
	}
	a.repo.DeleteExpiredSessions(ctx)
}
