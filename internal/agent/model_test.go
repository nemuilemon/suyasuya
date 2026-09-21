package agent

import (
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/genai"
	"os"
	"testing"
	"time"
)

type fakeTools struct {
	calls int
	names []string
}

func (f *fakeTools) Declarations() []*genai.FunctionDeclaration {
	return []*genai.FunctionDeclaration{{Name: "getSleepTime", Description: "Get this user's sleep time", Parameters: &genai.Schema{Type: genai.TypeObject, Properties: map[string]*genai.Schema{}}}}
}
func (f *fakeTools) Call(ctx context.Context, name string, args map[string]any) map[string]any {
	f.calls++
	f.names = append(f.names, name)
	return map[string]any{"output": map[string]any{"duration_minutes": 420, "date": "2026-09-20"}}
}
func response(parts ...*genai.Part) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Content: &genai.Content{Role: "model", Parts: parts}}}}
}
func TestFunctionRoundTrip(t *testing.T) {
	tools := &fakeTools{}
	signature := []byte("signed-thought")
	round := 0
	model := func(ctx context.Context, name string, history []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
		round++
		if round == 1 {
			return response(&genai.Part{ThoughtSignature: signature, FunctionCall: &genai.FunctionCall{ID: "call-1", Name: "getSleepTime"}}), nil
		}
		if round == 2 {
			if string(history[1].Parts[0].ThoughtSignature) != "signed-thought" {
				t.Fatal("signature lost")
			}
			reply := history[2].Parts[0].FunctionResponse
			if reply.ID != "call-1" || reply.Name != "getSleepTime" || reply.Response["output"] == nil {
				t.Fatal("function reply lost")
			}
			return response(&genai.Part{Text: "ready"}), nil
		}
		if len(config.Tools) != 0 || config.ResponseMIMEType != "application/json" {
			t.Fatal("final schema/tool separation")
		}
		return response(&genai.Part{Text: `{"body":"ok"}`}), nil
	}
	out, err := RunModel(context.Background(), model, tools, "job", "test", genai.Text("task"), &genai.GenerateContentConfig{ResponseMIMEType: "application/json"})
	if err != nil || out.Text() != `{"body":"ok"}` || tools.calls != 1 {
		t.Fatal("round trip failed", err)
	}
}
func TestToolBudgetAndCancellation(t *testing.T) {
	tools := &fakeTools{}
	models := 0
	model := func(ctx context.Context, name string, history []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
		models++
		if len(config.Tools) == 0 {
			return response(&genai.Part{Text: `{}`}), nil
		}
		return response(&genai.Part{FunctionCall: &genai.FunctionCall{Name: "getSleepTime"}}, &genai.Part{FunctionCall: &genai.FunctionCall{Name: "getSleepTime"}}, &genai.Part{FunctionCall: &genai.FunctionCall{Name: "getSleepTime"}}), nil
	}
	if _, err := RunModel(context.Background(), model, tools, "job", "test", genai.Text("task"), &genai.GenerateContentConfig{}); err != nil {
		t.Fatal(err)
	}
	if tools.calls != 8 || models > 5 {
		t.Fatal("budget exceeded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := models
	if _, err := RunModel(ctx, model, tools, "job", "test", nil, &genai.GenerateContentConfig{}); !errors.Is(err, context.Canceled) || before != models {
		t.Fatal("cancel ignored")
	}
}
func TestLiveFunctionCalling(t *testing.T) {
	if os.Getenv("LIVE_AGENT_TOOLS_TEST") != "1" {
		t.Skip("explicit live test required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: os.Getenv("GEMINI_API_KEY"), Backend: genai.BackendGeminiAPI})
	if err != nil {
		t.Fatal("client unavailable")
	}
	tools := &fakeTools{}
	out, err := RunModel(ctx, client.Models.GenerateContent, tools, "live-test", os.Getenv("GEMINI_MODEL"), genai.Text("必ず getSleepTime を呼び、取得した duration_minutes を minutes に入れて返してください。"), &genai.GenerateContentConfig{ResponseMIMEType: "application/json", ResponseJsonSchema: map[string]any{"type": "object", "required": []string{"minutes"}, "properties": map[string]any{"minutes": map[string]any{"type": "integer"}}}})
	if err != nil {
		t.Fatal("live function calling failed")
	}
	if tools.calls < 1 || out.Text() == "" {
		t.Fatal("no tool called")
	}
	var result struct {
		Minutes int `json:"minutes"`
	}
	if json.Unmarshal([]byte(out.Text()), &result) != nil || result.Minutes != 420 {
		t.Fatal("tool output was not used")
	}
	t.Logf("live tool calls: %d", tools.calls)
}
