package worker

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	"suyasuya/internal/platform"
	"suyasuya/internal/service/ocr"
)

type Runner struct {
	queue *redis.Client
	ocr   *ocr.Service
}

func New(queue *redis.Client, ocr *ocr.Service) *Runner { return &Runner{queue: queue, ocr: ocr} }
func (a *Runner) Run(ctx context.Context) error {
	log.Print("OCR worker ready")
	for ctx.Err() == nil {
		id, e := a.queue.BLMove(ctx, "ocr:pending", "ocr:processing", "RIGHT", "LEFT", 2*time.Second).Result()
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
			a.ocr.Process(ctx, id)
		}
		a.queue.LRem(ctx, "ocr:processing", 1, id)
		a.queue.Del(ctx, "queued:"+id)
	}
	return nil
}
