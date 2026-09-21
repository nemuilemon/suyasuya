package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/genai"
	"suyasuya/internal/domain"
	"suyasuya/internal/repository"
)

// Goal describes an outcome and available capabilities, not a prescribed workflow.
type Goal struct {
	Purpose      string   `json:"purpose"`
	Context      any      `json:"context"`
	AllowedTools []string `json:"allowed_tools"`
}
type Decision struct {
	Send   bool   `json:"send"`
	Body   string `json:"body"`
	Reason string `json:"reason"`
}
type Reasoner struct {
	repo   *repository.Store
	client *genai.Client
}

func NewReasoner(ctx context.Context, repo *repository.Store) (*Reasoner, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: os.Getenv("GEMINI_API_KEY"), Backend: genai.BackendGeminiAPI})
	return &Reasoner{repo: repo, client: client}, err
}
func (r *Reasoner) goal(ctx context.Context, task domain.Task) (Goal, error) {
	goal := Goal{AllowedTools: []string{"getSleepTime", "getPVTResults", "getUserSettings", "getNotificationHistory"}}
	var err error
	switch task.Kind {
	case "plan_reminder":
		var input domain.ReminderInput
		if json.Unmarshal(task.Input, &input) != nil || (input.Kind != "bedtime" && input.Kind != "pvt") {
			return goal, fmt.Errorf("invalid reminder context")
		}
		goal.Purpose, err = r.repo.ReminderPrompt(ctx, task.ID, task.UserID)
		goal.Context = input
	case "weekly_report":
		var input domain.WeeklyInput
		if json.Unmarshal(task.Input, &input) != nil {
			return goal, fmt.Errorf("invalid report context")
		}
		goal.Purpose, err = r.repo.WeeklyPrompt(ctx, task.ID, task.UserID)
		goal.Context = map[string]any{"week_start": input.Report.Start, "week_end": input.Report.End, "through": input.Report.Through, "timezone": input.Timezone, "delivery_requested": input.Deliver}
	default:
		return goal, fmt.Errorf("unsupported reasoning task")
	}
	return goal, err
}
func (r *Reasoner) Execute(ctx context.Context, task domain.Task) ([]byte, error) {
	goal, err := r.goal(ctx, task)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(goal)
	if err != nil {
		return nil, err
	}
	generation, cancel := context.WithTimeout(ctx, 100*time.Second)
	defer cancel()
	registry := NewToolRegistry(r.repo, task)
	registry.allowed = goal.AllowedTools
	output, err := RunModel(generation, r.client.Models.GenerateContent, registry, task.ID, task.Model, genai.Text(string(data)), &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{Parts: []*genai.Part{{Text: task.BasePrompt}, {Text: "目的に対して必要なtoolを選び、最終的にsend（送信要否）、body（ユーザーに伝える本文）、reason（判断理由）をJSONで返す。bodyは1500文字以内、reasonは500文字以内。取得できない事実を作らない。送信しない場合もreasonを残す。送信やDB更新はこの判断を受けたcontrolが担当する。"}}},
		ResponseMIMEType:  "application/json", ResponseJsonSchema: map[string]any{"type": "object", "required": []string{"send", "body", "reason"}, "properties": map[string]any{"send": map[string]any{"type": "boolean"}, "body": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}}},
	})
	if err != nil {
		return nil, err
	}
	var decision Decision
	if err = json.Unmarshal([]byte(output.Text()), &decision); err != nil {
		return nil, err
	}
	if err = validateDecision(decision); err != nil {
		return nil, err
	}
	return json.Marshal(decision)
}
func validateDecision(d Decision) error {
	if (d.Send && strings.TrimSpace(d.Body) == "") || utf8.RuneCountInString(d.Body) > 1500 || strings.TrimSpace(d.Reason) == "" || utf8.RuneCountInString(d.Reason) > 500 {
		return fmt.Errorf("invalid agent decision")
	}
	return nil
}
