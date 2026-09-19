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

func validateDeleteJobs(in domain.DeleteJobsInput) error {
	if in.Completed {
		if len(in.IDs) != 0 {
			return domain.Invalid("削除方法を1つ選んでください")
		}
		return nil
	}
	if len(in.IDs) == 0 || len(in.IDs) > 100 {
		return domain.Invalid("削除対象は1〜100件で選んでください")
	}
	for _, id := range in.IDs {
		if !platform.ValidID(id) {
			return domain.Invalid("job ID が不正です")
		}
	}
	return nil
}
func (a *Control) DeleteJobs(ctx context.Context, u domain.User, in domain.DeleteJobsInput) (any, error) {
	if err := validateDeleteJobs(in); err != nil {
		return nil, err
	}
	count, err := a.repo.DeleteJobs(ctx, u.ID, in.IDs, in.Completed)
	if err != nil {
		return nil, err
	}
	return map[string]int64{"deleted": count}, nil
}
