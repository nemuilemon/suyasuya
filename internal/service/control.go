package service

import (
	"context"
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"suyasuya/internal/domain"
	"suyasuya/internal/platform"
	"suyasuya/internal/repository"
)

type Control struct {
	repo      *repository.Store
	queue     *redis.Client
	model     string
	maxUpload int64
}

func New(repo *repository.Store, queue *redis.Client) *Control {
	return &Control{repo: repo, queue: queue, model: platform.Env("GEMINI_MODEL", "gemini-3.8-flash"), maxUpload: platform.Number("UPLOAD_MAX_BYTES", 10485760)}
}
func (a *Control) Health(ctx context.Context) error {
	if err := a.repo.Ping(ctx); err != nil {
		return err
	}
	return a.queue.Ping(ctx).Err()
}
func (a *Control) Seed(ctx context.Context) error {
	name := platform.Env("APP_USERNAME", "mouchan")
	password := os.Getenv("APP_PASSWORD")
	if len(password) < 12 {
		return fmt.Errorf("APP_PASSWORD must contain at least 12 characters")
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	tx, e := a.repo.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var id string
	_, e = tx.EnsureAdmin(ctx, name, string(hash))
	if e != nil {
		return e
	}
	if e = tx.UserID(ctx, name, &id); e != nil {
		return e
	}
	_, e = tx.EnsureSettings(ctx, id)
	if e != nil {
		return e
	}
	for k, body := range map[string]string{"job.extract_sleep": domain.OCRPrompt, "agent.loop": "承認済みの記録を根拠に判断する。材料が足りなければ保留する。不要な tool 呼び出しはしない。"} {
		_, e = tx.EnsurePrompt(ctx, k, body, id)
		if e != nil {
			return e
		}
	}
	return tx.Commit()
}
