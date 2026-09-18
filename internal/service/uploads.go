package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"

	"cloud.google.com/go/storage"

	"suyasuya/internal/domain"
	"suyasuya/internal/platform"
)

func gcs(ctx context.Context) (*storage.Client, error) { return storage.NewClient(ctx) }
func signURL(c *storage.Client, bucket, object, method, contentType string, generation int64) (string, error) {
	opts := &storage.SignedURLOptions{Scheme: storage.SigningSchemeV4, Method: method, GoogleAccessID: os.Getenv("GCS_SIGNER_SERVICE_ACCOUNT"), Expires: time.Now().Add(time.Duration(platform.Number("GCS_SIGNED_URL_TTL_SECONDS", 900)) * time.Second)}
	if method == "PUT" {
		opts.ContentType = contentType
		opts.Headers = []string{"x-goog-if-generation-match:0"}
	}
	if generation > 0 {
		opts.QueryParameters = url.Values{"generation": {strconv.FormatInt(generation, 10)}}
	}
	return c.Bucket(bucket).SignedURL(object, opts)
}
func (a *Control) CreateUpload(ctx context.Context, u domain.User, in domain.UploadInput) (any, error) {
	if in.Type != "image/png" && in.Type != "image/jpeg" && in.Type != "image/webp" {
		return nil, domain.Invalid("PNG / JPEG / WebP を選んでください")
	}
	if in.Size <= 0 || in.Size > a.maxUpload {
		return nil, domain.Invalid("画像のサイズ上限を超えています")
	}
	if _, e := time.Parse("2006-01-02", in.Date); e != nil {
		return nil, domain.Invalid("撮影日を指定してください")
	}
	bucket := os.Getenv("GCS_BUCKET")
	if bucket == "" || os.Getenv("GCS_SIGNER_SERVICE_ACCOUNT") == "" {
		return nil, domain.Error{Code: 503, Message: "Cloud Storage の設定が必要です"}
	}
	c, e := gcs(ctx)
	if e != nil {
		return nil, e
	}
	defer c.Close()
	id := platform.UUID()
	object := "uploads/" + u.ID + "/" + id
	signed, e := signURL(c, bucket, object, "PUT", in.Type, 0)
	if e != nil {
		return nil, e
	}
	expires := time.Now().Add(time.Duration(platform.Number("GCS_SIGNED_URL_TTL_SECONDS", 900)) * time.Second)
	_, e = a.repo.InsertUpload(ctx, id, u.ID, in.Type, bucket, object, in.Size, expires, in.Date)
	if e != nil {
		return nil, e
	}
	return map[string]any{"id": id, "url": signed, "headers": map[string]string{"Content-Type": in.Type, "x-goog-if-generation-match": "0"}}, nil
}

func (a *Control) CompleteUpload(ctx context.Context, u domain.User, id string) (any, error) {
	if !platform.ValidID(id) {
		return nil, domain.Invalid("upload ID が不正です")
	}
	tx, e := a.repo.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var bucket, object, contentType, status string
	var expected int64
	var expires time.Time
	e = tx.UploadForUpdate(ctx, id, u.ID, &bucket, &object, &contentType, &status, &expected, &expires)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, domain.Error{Code: 404, Message: "upload が見つかりません"}
	}
	if e != nil {
		return nil, e
	}
	var jobID string
	if status == "uploaded" {
		e = tx.UploadJob(ctx, u.ID, "ocr:"+id, &jobID)
		if e != nil {
			return nil, e
		}
		return map[string]string{"job_id": jobID}, nil
	}
	if status != "pending" {
		return nil, domain.Invalid("利用できない upload です")
	}
	// PUT 済みの完了通知は署名期限を過ぎても回復できる。長期放置分は明示的に失効。
	if time.Now().After(expires.Add(24 * time.Hour)) {
		return nil, domain.Invalid("upload の確認期限が切れました")
	}
	c, e := gcs(ctx)
	if e != nil {
		return nil, e
	}
	defer c.Close()
	attrs, e := c.Bucket(bucket).Object(object).Attrs(ctx)
	if errors.Is(e, storage.ErrObjectNotExist) {
		return nil, domain.Invalid("画像の PUT が完了していません")
	}
	if e != nil {
		return nil, e
	}
	if attrs.Size != expected || attrs.Size > a.maxUpload || attrs.ContentType != contentType {
		return nil, domain.Invalid("画像のサイズまたは種類が申告と一致しません")
	}
	_, e = tx.CompleteUpload(ctx, id, u.ID, attrs.Generation, attrs.Size)
	if e != nil {
		return nil, e
	}
	jobID = platform.UUID()
	_, e = tx.InsertOCRJob(ctx, jobID, u.ID, "ocr:"+id, id, a.model)
	if e != nil {
		return nil, e
	}
	// 実行前に一度固定。この MVP では作成時に OCR 指示の版を束縛する。
	res, e := tx.BindOCRPrompt(ctx, jobID)
	if e != nil {
		return nil, e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return nil, fmt.Errorf("active OCR prompt missing")
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	// Redis が落ちても job は DB に残り、sweeper が再投入する。
	a.enqueue(ctx, jobID)
	return map[string]string{"job_id": jobID}, nil
}

func (a *Control) ViewUpload(ctx context.Context, u domain.User, id string) (any, error) {
	if !platform.ValidID(id) {
		return nil, domain.Invalid("upload ID が不正です")
	}
	var bucket, object string
	var generation int64
	e := a.repo.ImageLocation(ctx, id, u.ID, &bucket, &object, &generation)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, domain.Error{Code: 404, Message: "画像が見つかりません"}
	}
	if e != nil {
		return nil, e
	}
	c, e := gcs(ctx)
	if e != nil {
		return nil, e
	}
	defer c.Close()
	signed, e := signURL(c, bucket, object, "GET", "", generation)
	if e != nil {
		return nil, e
	}
	return map[string]string{"url": signed}, nil
}
