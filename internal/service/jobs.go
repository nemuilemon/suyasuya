package service

import (
	"context"

	"suyasuya/internal/domain"
	"suyasuya/internal/platform"
)

func (a *Control) Jobs(ctx context.Context, u domain.User) (any, error) {
	out, e := a.repo.Jobs(ctx, u.ID)
	if e != nil {
		return nil, e
	}
	return out, nil
}

func (a *Control) Retry(ctx context.Context, u domain.User, id string) (any, error) {
	if !platform.ValidID(id) {
		return nil, domain.Invalid("job ID が不正です")
	}
	res, e := a.repo.RetryJob(ctx, id, u.ID)
	if e != nil {
		return nil, e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return nil, domain.Error{Code: 409, Message: "再試行できる job がありません"}
	}
	a.enqueue(ctx, id)
	return map[string]bool{"ok": true}, nil
}

func (a *Control) CancelJob(ctx context.Context, u domain.User, id string) (any, error) {
	if !platform.ValidID(id) {
		return nil, domain.Invalid("job ID が不正です")
	}
	res, e := a.repo.CancelJob(ctx, id, u.ID)
	if e != nil {
		return nil, e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return nil, domain.Error{Code: 409, Message: "中止できる job がありません"}
	}
	return map[string]bool{"ok": true}, nil
}
