package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDiscordDelivery(t *testing.T) {
	for _, tc := range []struct {
		name           string
		status         int
		response       string
		success, retry bool
	}{
		{"receipt", 200, `{"id":"1234"}`, true, false},
		{"rate limit", 429, `{"retry_after":1.5}`, false, true},
		{"invalid rate limit", 429, `{"retry_after":-1}`, false, false},
		{"server failure", 500, `{}`, false, false},
		{"bad webhook", 404, `{}`, false, false},
		{"unconfirmed", 200, `{}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDiscord()
			d.Client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Query().Get("wait") != "true" {
					t.Fatal("receipt not requested")
				}
				var payload struct {
					Content  string `json:"content"`
					Username string `json:"username"`
					Allowed  struct {
						Parse []string `json:"parse"`
					} `json:"allowed_mentions"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if payload.Username != "custom agent" || payload.Content != "test @everyone" || payload.Allowed.Parse == nil || len(payload.Allowed.Parse) != 0 {
					t.Fatal("mention suppression missing")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.response))}, nil
			})
			result := d.Send(context.Background(), "https://discord.com/api/webhooks/1/secret", "test @everyone", "custom agent")
			if (result.MessageID != "") != tc.success || (result.RetryAfter > 0) != tc.retry {
				t.Fatalf("unexpected result %+v", result)
			}
			if tc.retry && result.RetryAfter < 1500*time.Millisecond {
				t.Fatal("retried too early")
			}
		})
	}
}
func TestDiscordErrorDoesNotLeakURL(t *testing.T) {
	d := NewDiscord()
	d.Client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) { return nil, errors.New(r.URL.String()) })
	result := d.Send(context.Background(), "https://discord.com/api/webhooks/1/secret", "test", "")
	if strings.Contains(result.Error, "secret") || result.RetryAfter != 0 {
		t.Fatal("leaked credentials or ambiguous retry")
	}
	if d.Client.CheckRedirect == nil {
		t.Fatal("redirects must be blocked")
	}
}

func TestWebhookNameIsNotOverriddenWhenEmpty(t *testing.T) {
	d := NewDiscord()
	d.Client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if _, ok := body["username"]; ok {
			t.Fatal("hardcoded persona override")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"123"}`))}, nil
	})
	if result := d.Send(context.Background(), "https://discord.com/api/webhooks/1/test", "test", ""); result.MessageID != "123" {
		t.Fatal(result)
	}
}
