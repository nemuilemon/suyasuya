// Package notifier owns delivery, never agent decisions or credentials storage.
package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

type Result struct {
	MessageID  string
	RetryAfter time.Duration
	Error      string
}
type Notifier interface {
	Send(context.Context, string, string) Result
}
type Discord struct{ Client *http.Client }

func NewDiscord() *Discord {
	return &Discord{Client: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func ValidURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "discord.com" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.RawPath == "" && regexp.MustCompile(`^/api/webhooks/[0-9]+/[A-Za-z0-9_-]+$`).MatchString(u.Path)
}
func (d *Discord) Send(ctx context.Context, webhook, body string) Result {
	if !ValidURL(webhook) {
		return Result{Error: "Webhook URL を確認してください"}
	}
	payload, _ := json.Marshal(map[string]any{"content": body, "username": "ありす / suyasuya", "allowed_mentions": map[string]any{"parse": []string{}}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook+"?wait=true", bytes.NewReader(payload))
	if err != nil {
		return Result{Error: "送信リクエストを作成できませんでした"}
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := d.Client.Do(req)
	// HTTP errors can contain the credential-bearing URL. Never persist or log them.
	if err != nil {
		return Result{Error: "送信結果を確認できませんでした。重複防止のため自動再送しません"}
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusTooManyRequests {
		var limit struct {
			RetryAfter float64 `json:"retry_after"`
		}
		if json.NewDecoder(io.LimitReader(res.Body, 8192)).Decode(&limit) == nil && limit.RetryAfter > 0 && limit.RetryAfter <= 900 {
			return Result{RetryAfter: time.Duration(limit.RetryAfter*float64(time.Second)) + time.Second, Error: "Discord の送信制限により待機中"}
		}
		return Result{Error: "Discord の送信制限。再送時刻を取得できませんでした"}
	}
	if res.StatusCode != http.StatusOK {
		return Result{Error: fmt.Sprintf("Discord 送信失敗 (HTTP %d)", res.StatusCode)}
	}
	var receipt struct {
		ID string `json:"id"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&receipt) != nil || receipt.ID == "" {
		return Result{Error: "送信確認を読み取れませんでした。重複防止のため自動再送しません"}
	}
	return Result{MessageID: receipt.ID}
}
