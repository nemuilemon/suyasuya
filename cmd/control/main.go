package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"suyasuya/internal/handler"
	"suyasuya/internal/platform"
	"suyasuya/internal/repository"
	"suyasuya/internal/router"
	"suyasuya/internal/service"
)

func main() {
	if e := run(); e != nil {
		log.Fatal(platform.SafeError(e))
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, e := repository.Open(ctx, "CONTROL")
	if e != nil {
		return e
	}
	defer db.Close()
	queue := redis.NewClient(&redis.Options{Addr: platform.Env("REDIS_ADDR", "redis:6379")})
	defer queue.Close()
	svc := service.New(repository.New(db), queue)
	if e = svc.Seed(ctx); e != nil {
		return e
	}
	go svc.Sweep(ctx)
	go svc.NotificationsLoop(ctx)
	server := &http.Server{Addr: ":8080", Handler: router.New(handler.New(svc)), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	log.Print("control listening on :8080")
	e = server.ListenAndServe()
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return e

}
