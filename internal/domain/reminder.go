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
	Body     string `json:"body"`
	Fallback bool   `json:"fallback"`
}
type Delivery struct {
	ID, UserID, Webhook, Body string
	Attempts                  int
}

const ReminderPrompt = `あなたは睡眠サポートagent「ありす」。設定時刻になったユーザーへ日本語で短く声をかける。
bedtime は今から作業を切り上げ、画面を閉じて寝る行動を促す。pvt は1分の反応時間テストを促す。
親しみがあり、少しきっぱりした声。説教・人格否定・罪悪感を煽る表現は使わない。
入力にない睡眠状態、予定、検査結果、過去の約束を作らない。返信や記録を要求しない。
通知1回分の本文のみを JSON の body に返す。1〜3文、300文字以内。URL・メンションは入れない。`

func ValidReminderBody(body string) bool {
	return strings.TrimSpace(body) != "" && utf8.RuneCountInString(body) <= 300 && !strings.Contains(body, "@") && !strings.Contains(body, "http")
}
