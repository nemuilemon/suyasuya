package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"suyasuya/internal/domain"
	"suyasuya/internal/platform"
)

func parseRecord(in domain.RecordInput) (*time.Time, *time.Time, error) {
	loc, e := time.LoadLocation(in.Timezone)
	if e != nil {
		return nil, nil, domain.Invalid("タイムゾーンを確認してください")
	}
	if _, e = time.Parse("2006-01-02", in.Date); e != nil {
		return nil, nil, domain.Invalid("記録の日付を確認してください")
	}
	parse := func(s string) (*time.Time, error) {
		if s == "" {
			return nil, nil
		}
		t, e := time.ParseInLocation("2006-01-02T15:04", s, loc)
		if e != nil {
			return nil, domain.Invalid("開始・終了日時を確認してください")
		}
		return &t, nil
	}
	start, e := parse(in.Start)
	if e != nil {
		return nil, nil, e
	}
	end, e := parse(in.End)
	if e != nil {
		return nil, nil, e
	}
	if in.Kind != "main" && in.Kind != "nap" {
		return nil, nil, domain.Invalid("主睡眠か昼寝を選んでください")
	}
	if in.Status != "approved" && in.Status != "rejected" && in.Status != "deleted" {
		return nil, nil, domain.Invalid("記録の状態が不正です")
	}
	if in.Duration != nil && (*in.Duration < 0 || *in.Duration > 1440) {
		return nil, nil, domain.Invalid("睡眠時間は0〜1440分で入力してください")
	}
	if start != nil && end != nil {
		if !end.After(*start) {
			return nil, nil, domain.Invalid("終了は開始より後にしてください")
		}
		if in.Duration != nil && time.Duration(*in.Duration)*time.Minute > end.Sub(*start) {
			return nil, nil, domain.Invalid("睡眠時間が記録区間を超えています")
		}
	}
	if in.Status == "approved" && start == nil && end == nil && in.Duration == nil {
		return nil, nil, domain.Invalid("日時または睡眠時間を入力してください")
	}
	return start, end, nil
}
func (a *Control) Records(ctx context.Context, u domain.User) (any, error) {
	out, e := a.repo.Records(ctx, u.ID)
	if e != nil {
		return nil, e
	}
	return out, nil
}

func (a *Control) History(ctx context.Context, u domain.User, id string) (any, error) {
	if !platform.ValidID(id) {
		return nil, domain.Invalid("記録 ID が不正です")
	}
	out, e := a.repo.History(ctx, id, u.ID)
	if e != nil {
		return nil, e
	}
	return out, nil
}

func (a *Control) SaveRecord(ctx context.Context, u domain.User, in domain.RecordInput) (any, error) {
	if !platform.ValidID(in.SubmissionID) {
		return nil, domain.Invalid("送信 ID が不正です")
	}
	start, end, e := parseRecord(in)
	if e != nil {
		return nil, e
	}
	tx, e := a.repo.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	// 同じ request の並行送信も直列化する。
	if _, e = tx.LockSubmission(ctx, u.ID+in.SubmissionID); e != nil {
		return nil, e
	}
	var existing string
	e = tx.SubmissionRecord(ctx, u.ID, in.SubmissionID, &existing)
	if e == nil {
		return map[string]string{"id": existing}, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return nil, e
	}
	source := "manual"
	var job, watch sql.NullString
	var candidate sql.NullInt64
	rev := 1
	if in.ID != "" {
		if !platform.ValidID(in.ID) {
			return nil, domain.Invalid("記録 ID が不正です")
		}
		var currentStatus string
		e = tx.CurrentRecord(ctx, in.ID, u.ID, &rev, &source, &job, &candidate, &watch, &currentStatus)
		if errors.Is(e, sql.ErrNoRows) {
			return nil, domain.Error{Code: 404, Message: "記録が見つかりません"}
		}
		if e != nil {
			return nil, e
		}
		if rev != in.Revision || currentStatus == "deleted" {
			return nil, domain.Error{Code: 409, Message: "記録が変更されています。再読み込みしてください"}
		}
		if _, e = tx.RetireRecord(ctx, in.ID, rev, u.ID); e != nil {
			return nil, e
		}
		rev++
	} else {
		if in.Status == "deleted" {
			return nil, domain.Invalid("削除対象がありません")
		}
		in.ID = platform.UUID()
		if in.JobID != "" {
			if !platform.ValidID(in.JobID) || in.CandidateIndex == nil || *in.CandidateIndex < 0 {
				return nil, domain.Invalid("読み取り候補が不正です")
			}
			var output []byte
			e = tx.CandidateOutput(ctx, in.JobID, u.ID, &output)
			if errors.Is(e, sql.ErrNoRows) {
				return nil, domain.Invalid("読み取り済みの候補がありません")
			}
			if e != nil {
				return nil, e
			}
			var result domain.OCRResult
			if e = json.Unmarshal(output, &result); e != nil {
				return nil, e
			}
			if *in.CandidateIndex >= len(result.Records) {
				return nil, domain.Invalid("候補がありません")
			}
			var already bool
			e = tx.CandidateReviewed(ctx, u.ID, in.JobID, *in.CandidateIndex, &already)
			if e != nil {
				return nil, e
			}
			if already {
				return nil, domain.Error{Code: 409, Message: "この候補は確認済みです"}
			}
			source = "image"
			job = sql.NullString{String: in.JobID, Valid: true}
			candidate = sql.NullInt64{Int64: int64(*in.CandidateIndex), Valid: true}
		} else if in.Status == "rejected" {
			return nil, domain.Invalid("却下対象がありません")
		}
	}
	_, e = tx.InsertRecord(ctx, in.ID, rev, u.ID, in.Status, in.Kind, in.Date, in.Timezone, start, end, in.Duration, source, job, candidate, watch, in.SubmissionID)
	if e != nil {
		return nil, e
	}
	_, e = tx.InvalidateEvaluations(ctx, u.ID, in.ID)
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return map[string]any{"id": in.ID, "revision": rev}, nil
}
