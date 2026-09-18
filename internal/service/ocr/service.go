package ocr

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/genai"

	"suyasuya/internal/domain"
	"suyasuya/internal/platform"
	"suyasuya/internal/repository"
)

type Service struct {
	repo      *repository.Store
	client    *genai.Client
	maxUpload int64
}

func New(ctx context.Context, repo *repository.Store) (*Service, error) {
	if os.Getenv("GEMINI_API_KEY") == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY is required")
	}
	client, e := genai.NewClient(ctx, &genai.ClientConfig{APIKey: os.Getenv("GEMINI_API_KEY"), Backend: genai.BackendGeminiAPI})
	if e != nil {
		return nil, e
	}
	return &Service{repo: repo, client: client, maxUpload: platform.Number("UPLOAD_MAX_BYTES", 10485760)}, nil
}
func ocrSchema() map[string]any {
	nullableText := map[string]any{"type": []string{"string", "null"}}
	numeric := func() any {
		return map[string]any{"type": []string{"integer", "null"}}
	}
	props := map[string]any{"sleep_kind": map[string]any{"type": []string{"string", "null"}, "description": "main, nap, or null when unknown"}, "start_time": nullableText, "end_time": nullableText}
	required := []string{"sleep_kind", "start_time", "end_time"}
	for _, k := range []string{"duration_minutes", "sleep_score", "awake_minutes", "restlessness_minutes", "rem_minutes", "light_minutes", "deep_minutes"} {
		props[k] = numeric()
		required = append(required, k)
	}
	return map[string]any{"type": "object", "required": []string{"app", "displayed_date", "records", "uncertainties"}, "properties": map[string]any{"app": nullableText, "displayed_date": nullableText, "records": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": props, "required": required}}, "uncertainties": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}}
}
func validateOCR(out *domain.OCRResult) error {
	if len(out.Records) == 0 || len(out.Records) > 20 {
		return fmt.Errorf("sleep records were not found")
	}
	for _, r := range out.Records {
		if r.Kind != nil && *r.Kind != "main" && *r.Kind != "nap" {
			return fmt.Errorf("invalid kind")
		}
		for _, s := range []*string{r.Start, r.End} {
			if s != nil {
				if !regexp.MustCompile(`^\d{1,2}:\d{2}$`).MatchString(*s) {
					return fmt.Errorf("invalid time")
				}
				t, e := time.Parse("15:04", *s)
				if e != nil {
					return e
				}
				*s = t.Format("15:04")
			}
		}
		for _, v := range []*int{r.Duration, r.Awake, r.Restless, r.REM, r.Light, r.Deep} {
			if v != nil && (*v < 0 || *v > 1440) {
				return fmt.Errorf("invalid minutes")
			}
		}
		if r.Score != nil && (*r.Score < 0 || *r.Score > 100) {
			return fmt.Errorf("invalid score")
		}
	}
	return nil
}
func (a *Service) Process(parent context.Context, id string) {
	lease := platform.UUID()
	var uid, upload, model string
	e := a.repo.ClaimJob(parent, id, lease, &uid, &upload, &model)
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
				if a.repo.JobCancelled(ctx, id, uid, &requested) == nil && requested {
					cancel()
					return
				}
			}
		}
	}()
	output, e := a.extract(ctx, uid, upload, id, model)
	cleanup, cancelCleanup := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelCleanup()
	if e != nil {
		log.Printf("OCR %s: %s", id, platform.SafeError(e))
		a.repo.FailJob(cleanup, id, uid, lease, "画像の読み取りに失敗しました。設定や画像を確認して再試行してください。")
		return
	}
	tx, e := a.repo.Begin(cleanup)
	if e != nil {
		return
	}
	defer tx.Rollback()
	result, e := tx.CompleteJob(cleanup, id, uid, lease)
	if e != nil {
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		tx.Rollback()
		a.repo.CancelClaim(cleanup, id, uid, lease)
		return
	}
	_, e = tx.InsertResult(cleanup, id, uid, string(output))
	if e != nil {
		return
	}
	if e = tx.Commit(); e != nil {
		log.Printf("complete %s: %s", id, platform.SafeError(e))
	}
}
func (a *Service) extract(ctx context.Context, uid, upload, id, model string) ([]byte, error) {
	data, mime, e := a.readImage(ctx, uid, upload)
	if e != nil {
		return nil, e
	}
	var prompt string
	e = a.repo.JobPrompt(ctx, id, uid, &prompt)
	if e != nil {
		return nil, e
	}
	response, e := a.client.Models.GenerateContent(ctx, model, []*genai.Content{{Role: "user", Parts: []*genai.Part{{InlineData: &genai.Blob{Data: data, MIMEType: mime}}}}}, &genai.GenerateContentConfig{SystemInstruction: &genai.Content{Parts: []*genai.Part{{Text: prompt}}}, ResponseMIMEType: "application/json", ResponseJsonSchema: ocrSchema()})
	if e != nil {
		return nil, e
	}
	var out domain.OCRResult
	if e = json.Unmarshal([]byte(response.Text()), &out); e != nil {
		return nil, e
	}
	if e = validateOCR(&out); e != nil {
		return nil, e
	}
	return json.Marshal(out)
}
func (a *Service) readImage(ctx context.Context, uid, upload string) ([]byte, string, error) {
	var bucket, object, contentType string
	var generation int64
	e := a.repo.ImageForOCR(ctx, upload, uid, &bucket, &object, &contentType, &generation)
	if e != nil {
		return nil, "", e
	}
	c, e := gcs(ctx)
	if e != nil {
		return nil, "", e
	}
	defer c.Close()
	reader, e := c.Bucket(bucket).Object(object).Generation(generation).NewReader(ctx)
	if e != nil {
		return nil, "", e
	}
	defer reader.Close()
	data, e := io.ReadAll(io.LimitReader(reader, a.maxUpload+1))
	if e != nil {
		return nil, "", e
	}
	if int64(len(data)) > a.maxUpload {
		return nil, "", fmt.Errorf("image too large")
	}
	if http.DetectContentType(data) != contentType {
		return nil, "", fmt.Errorf("image content does not match MIME type")
	}
	return data, contentType, nil
}
func gcs(ctx context.Context) (*storage.Client, error) { return storage.NewClient(ctx) }
