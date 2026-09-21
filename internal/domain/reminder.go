package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

type ReminderSchedule struct {
	UserID       string
	Timezone     string
	Bedtime      string
	PVT          string
	SnoozedUntil *time.Time
}
type ReminderInput struct {
	Kind     string    `json:"kind"`
	Timezone string    `json:"timezone"`
	Target   string    `json:"target"`
	DueAt    time.Time `json:"due_at"`
}
type ReminderResult struct {
	Send     *bool  `json:"send,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Body     string `json:"body"`
	Fallback bool   `json:"fallback"`
}
type Delivery struct {
	ID, UserID, Webhook, Body, AgentName string
	Attempts                             int
}

const ReminderPrompt = `ユーザーが設定した時刻に合わせ、定時の就寝（bedtime）または1分PVTの実施（pvt）を支える。必要なデータをtoolで確認し、今伝える内容と送信の要否を判断する。`

func ValidReminderBody(body string) bool {
	return strings.TrimSpace(body) != "" && utf8.RuneCountInString(body) <= 1500 && !strings.Contains(body, "@") && !strings.Contains(body, "http")
}
