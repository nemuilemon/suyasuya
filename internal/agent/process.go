package agent

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"suyasuya/internal/domain"
	"suyasuya/internal/platform"
)

// Handler processes one task. Lease, cancellation and persistence are shared.
type Handler func(context.Context, domain.Task) ([]byte, error)

func (a *Runner) execute(ctx context.Context, task domain.Task) ([]byte, error) {
	handler, ok := a.handlers[task.Kind]
	if !ok {
		return nil, fmt.Errorf("unsupported task kind: %s", task.Kind)
	}
	return handler(ctx, task)
}
func (a *Runner) Process(parent context.Context, id string) {
	lease := platform.UUID()
	var job domain.Task
	job.ID = id
	e := a.repo.ClaimTask(parent, id, lease, &job)
	if errors.Is(e, sql.ErrNoRows) {
		return
	}
	if e != nil {
		log.Printf("claim %s: %s", id, platform.SafeError(e))
		return
	}
	ctx, cancel := context.WithTimeout(parent, 120*time.Second)
	defer cancel()
	// 定期確認で control からの中止を拾う。保存済み状態は最後にも照合する。
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				var requested bool
				if a.repo.JobCancelled(ctx, id, job.UserID, &requested) == nil && requested {
					cancel()
					return
				}
			}
		}
	}()
	job.BasePrompt, e = a.repo.TaskBasePrompt(ctx, job.ID, job.UserID)
	var output []byte
	if e == nil {
		output, e = a.execute(ctx, job)
	}
	cleanup, cancelCleanup := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelCleanup()
	if e != nil {
		log.Printf("task %s: %s", id, platform.SafeError(e))
		a.repo.FailJob(cleanup, id, job.UserID, lease, "タスクの処理に失敗しました。設定や入力を確認して再試行してください。")
		return
	}
	tx, e := a.repo.Begin(cleanup)
	if e != nil {
		return
	}
	defer tx.Rollback()
	result, e := tx.CompleteJob(cleanup, id, job.UserID, lease)
	if e != nil {
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		tx.Rollback()
		a.repo.CancelClaim(cleanup, id, job.UserID, lease)
		return
	}
	_, e = tx.InsertResult(cleanup, id, job.UserID, string(output))
	if e != nil {
		return
	}
	if e = tx.Commit(); e != nil {
		log.Printf("complete %s: %s", id, platform.SafeError(e))
	}
}
