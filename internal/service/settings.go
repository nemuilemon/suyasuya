package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"suyasuya/internal/domain"
)

func (a *Control) Settings(ctx context.Context, u domain.User) (any, error) {
	var b []byte
	e := a.repo.Settings(ctx, u.ID, &b)
	if e != nil {
		return nil, e
	}
	return json.RawMessage(b), nil
}

func (a *Control) SaveSettings(ctx context.Context, u domain.User, in domain.SettingsInput) (any, error) {
	if _, e := time.LoadLocation(in.Timezone); e != nil {
		return nil, domain.Invalid("タイムゾーンを確認してください")
	}
	for _, s := range []string{in.PVT, in.Bedtime} {
		if s != "" {
			if _, e := time.Parse("15:04", s); e != nil {
				return nil, domain.Invalid("時刻を確認してください")
			}
		}
	}
	_, e := a.repo.SaveSettings(ctx, u.ID, in.Timezone, in.PVT, in.Bedtime)
	if e != nil {
		return nil, e
	}
	return a.Settings(ctx, u)
}

func (a *Control) Prompts(ctx context.Context, u domain.User) (any, error) {
	if !u.Admin {
		return nil, domain.Error{Code: 403, Message: "管理者のみ利用できます"}
	}
	out, e := a.repo.Prompts(ctx)
	if e != nil {
		return nil, e
	}
	return out, nil
}

func (a *Control) SavePrompt(ctx context.Context, u domain.User, in domain.PromptInput) (any, error) {
	if !u.Admin {
		return nil, domain.Error{Code: 403, Message: "管理者のみ利用できます"}
	}
	if in.Key != "job.extract_sleep" && in.Key != "agent.loop" {
		return nil, domain.Invalid("未対応のプロンプトです")
	}
	if len(strings.TrimSpace(in.Body)) == 0 || len(in.Body) > 32000 {
		return nil, domain.Invalid("本文は1〜32000 bytesで指定してください")
	}
	tx, e := a.repo.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	_, e = tx.LockPrompt(ctx, in.Key)
	if e != nil {
		return nil, e
	}
	_, e = tx.RetirePrompt(ctx, in.Key)
	if e != nil {
		return nil, e
	}
	_, e = tx.InsertPrompt(ctx, in.Key, in.Body, u.ID)
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return a.Prompts(ctx, u)
}
