package reminder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/genai"
	"suyasuya/internal/domain"
	"suyasuya/internal/repository"
)

type Service struct {
	repo   *repository.Store
	client *genai.Client
}

func New(ctx context.Context, repo *repository.Store) (*Service, error) {
	c, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: os.Getenv("GEMINI_API_KEY"), Backend: genai.BackendGeminiAPI})
	return &Service{repo: repo, client: c}, err
}
func (s *Service) Execute(ctx context.Context, task domain.Task) ([]byte, error) {
	var in domain.ReminderInput
	if json.Unmarshal(task.Input, &in) != nil || (in.Kind != "bedtime" && in.Kind != "pvt") {
		return nil, fmt.Errorf("invalid reminder input")
	}
	prompt, err := s.repo.ReminderPrompt(ctx, task.ID, task.UserID)
	if err != nil {
		return nil, err
	}
	generation, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	response, err := s.client.Models.GenerateContent(generation, task.Model, genai.Text(string(task.Input)), &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{Parts: []*genai.Part{{Text: prompt}}}, ResponseMIMEType: "application/json",
		ResponseJsonSchema: map[string]any{"type": "object", "required": []string{"body"}, "properties": map[string]any{"body": map[string]any{"type": "string"}}},
	})
	var out domain.ReminderResult
	if err == nil {
		err = json.Unmarshal([]byte(response.Text()), &out)
	}
	out.Body = strings.TrimSpace(out.Body)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil || !domain.ValidReminderBody(out.Body) {
		out.Fallback = true
		out.Body = "もう寝る時間だよ。今日はここまで。画面を閉じて、寝る準備をしよう。"
		if in.Kind == "pvt" {
			out.Body = "反応時間を確かめる時間だよ。手が空いたら、1分の PVT をやってみて。"
		}
	}
	return json.Marshal(out)
}
