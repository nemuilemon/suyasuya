package service

import (
	"context"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"suyasuya/internal/domain"
	"suyasuya/internal/platform"
)

func (a *Control) Login(ctx context.Context, in domain.LoginInput) (string, error) {
	// Redis の短時間制限。proxy の任意ヘッダーから IP を採らない。
	key := "login:" + platform.Digest(strings.ToLower(in.Username))
	n, e := a.queue.Incr(ctx, key).Result()
	if e != nil {
		return "", e
	}
	if n == 1 {
		a.queue.Expire(ctx, key, time.Minute)
	}
	if n > 10 {
		return "", domain.Error{Code: 429, Message: "少し待ってからログインしてください"}
	}
	var id, hash string
	e = a.repo.LoginCredentials(ctx, in.Username, &id, &hash)
	if e != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		return "", domain.Error{Code: 401, Message: "ユーザー名またはパスワードが違います"}
	}
	if e = a.EnsurePersonalAgent(ctx, id); e != nil {
		return "", e
	}
	t := platform.Token()
	_, e = a.repo.CreateSession(ctx, platform.Digest(t), id)
	if e != nil {
		return "", e
	}
	return t, nil
}
func (a *Control) Authenticate(ctx context.Context, token string) (domain.User, error) {
	var u domain.User
	e := a.repo.SessionUser(ctx, platform.Digest(token), &u.ID, &u.Name, &u.Admin)
	return u, e
}
func (a *Control) Logout(ctx context.Context, u domain.User, token string) error {
	_, e := a.repo.DeleteSession(ctx, platform.Digest(token), u.ID)
	return e
}
func (a *Control) Me(u domain.User) any {
	return map[string]any{"user": u, "storage_configured": os.Getenv("GCS_BUCKET") != "" && os.Getenv("GCS_SIGNER_SERVICE_ACCOUNT") != ""}
}
