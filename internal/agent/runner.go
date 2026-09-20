package agent

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	"suyasuya/internal/platform"
	"suyasuya/internal/repository"
)

type Runner struct {
	queue    *redis.Client
	repo     *repository.Store
	handlers map[string]Handler
}

func New(queue *redis.Client, repo *repository.Store, handlers map[string]Handler) *Runner {
	return &Runner{queue: queue, repo: repo, handlers: handlers}
}
func (a *Runner) Run(ctx context.Context) error {
	log.Print("agent ready")
	for ctx.Err() == nil {
		id, e := a.queue.BLMove(ctx, "tasks:pending", "tasks:processing", "RIGHT", "LEFT", 2*time.Second).Result()
		if errors.Is(e, redis.Nil) {
			continue
		}
		if e != nil {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
			continue
		}
		if platform.ValidID(id) {
			a.Process(ctx, id)
		}
		a.queue.LRem(ctx, "tasks:processing", 1, id)
		a.queue.Del(ctx, "queued:"+id)
	}
	return nil
}
