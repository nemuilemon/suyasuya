package domain

import (
	"encoding/json"
	"time"
)

type User struct {
	ID    string `json:"id"`
	Name  string `json:"username"`
	Admin bool   `json:"is_admin"`
}

type RecordInput struct {
	ID             string `json:"id"`
	Revision       int    `json:"revision"`
	SubmissionID   string `json:"submission_id"`
	JobID          string `json:"source_job_id"`
	CandidateIndex *int   `json:"candidate_index"`
	Status         string `json:"status"`
	Kind           string `json:"sleep_kind"`
	Date           string `json:"sleep_date"`
	Timezone       string `json:"timezone"`
	Start          string `json:"started_at"`
	End            string `json:"ended_at"`
	Duration       *int   `json:"duration_minutes"`
}

type PVTInput struct {
	ID        string    `json:"id"`
	Started   time.Time `json:"started_at"`
	Finished  time.Time `json:"finished_at"`
	Timezone  string    `json:"timezone"`
	Device    string    `json:"device"`
	Input     string    `json:"input_method"`
	Completed bool      `json:"completed"`
	Trials    []Trial   `json:"trials"`
}

type Trial struct {
	Stimulus *float64 `json:"stimulus_at_ms"`
	Response float64  `json:"response_at_ms"`
	RT       *float64 `json:"rt_ms"`
	False    bool     `json:"false_start"`
}

type Metrics struct {
	Median *float64 `json:"median_rt_ms"`
	Lapses int      `json:"lapses"`
	Slow   *float64 `json:"slowest10_mean_ms"`
	False  int      `json:"false_starts"`
}

type OCRRecord struct {
	Kind     *string `json:"sleep_kind"`
	Start    *string `json:"start_time"`
	End      *string `json:"end_time"`
	Duration *int    `json:"duration_minutes"`
	Score    *int    `json:"sleep_score"`
	Awake    *int    `json:"awake_minutes"`
	Restless *int    `json:"restlessness_minutes"`
	REM      *int    `json:"rem_minutes"`
	Light    *int    `json:"light_minutes"`
	Deep     *int    `json:"deep_minutes"`
}

type OCRResult struct {
	App           *string     `json:"app"`
	Date          *string     `json:"displayed_date"`
	Records       []OCRRecord `json:"records"`
	Uncertainties []string    `json:"uncertainties"`
}

type SettingsInput struct {
	AgentName            *string `json:"agent_name"`
	WeeklyEnabled        *bool   `json:"weekly_report_enabled"`
	WeeklyWeekday        *int    `json:"weekly_report_weekday"`
	WeeklyTime           *string `json:"weekly_report_time"`
	NotificationsEnabled *bool   `json:"notifications_enabled"`
	Webhook              *string `json:"discord_webhook_url"`
	ClearWebhook         bool    `json:"clear_webhook"`
	Timezone             string  `json:"timezone"`
	PVT                  string  `json:"pvt_reminder_time"`
	Bedtime              string  `json:"target_bedtime"`
}

type PromptInput struct {
	Key  string `json:"config_key"`
	Body string `json:"body"`
}

type UploadInput struct {
	Type string `json:"content_type"`
	Size int64  `json:"size_bytes"`
	Date string `json:"capture_date"`
}

type LoginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// DeleteJobsInput deletes either selected tasks or all fully reviewed tasks.
type DeleteJobsInput struct {
	IDs       []string `json:"ids"`
	Completed bool     `json:"completed"`
}

// Task carries the owner and input claimed from PostgreSQL, never from the queue.
type Task struct {
	ID, UserID, Kind, UploadID, Model string
	BasePrompt                        string
	Input                             json.RawMessage
}

// Instructions places the shared base before the task-specific instructions.
func (t Task) Instructions(specific string) string { return t.BasePrompt + "\n\n" + specific }
