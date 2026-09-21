package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/redis/go-redis/v9"

	"suyasuya/internal/agent"
	"suyasuya/internal/platform"
	"suyasuya/internal/repository"
	"suyasuya/internal/service/ocr"
)

func main() {
	if e := run(); e != nil {
		log.Fatal(platform.SafeError(e))
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, e := repository.Open(ctx, "AGENT")
	if e != nil {
		return e
	}
	defer db.Close()
	queue := redis.NewClient(&redis.Options{Addr: platform.Env("REDIS_ADDR", "redis:6379")})
	defer queue.Close()
	svc, e := ocr.New(ctx, repository.New(db))
	if e != nil {
		return e
	}
	reasoner, e := agent.NewReasoner(ctx, repository.New(db))
	if e != nil {
		return e
	}
	return agent.New(queue, repository.New(db), map[string]agent.Handler{
		"extract_sleep": svc.Execute,
		"plan_reminder": reasoner.Execute,
		"weekly_report": reasoner.Execute,
	}).Run(ctx)

}
