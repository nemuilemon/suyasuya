package service

import "testing"

func TestValidWebhook(t *testing.T) {
	for _, tc := range []struct {
		url   string
		valid bool
	}{
		{"https://discord.com/api/webhooks/123/token_A-1", true},
		{"http://discord.com/api/webhooks/123/token", false},
		{"https://discord.com.evil.test/api/webhooks/123/token", false},
		{"https://user@discord.com/api/webhooks/123/token", false},
		{"https://discord.com/api/webhooks/123/token?x=1", false},
		{"https://localhost/api/webhooks/123/token", false},
	} {
		if validWebhook(tc.url) != tc.valid {
			t.Errorf("unexpected validation for %s", tc.url)
		}
	}
}
