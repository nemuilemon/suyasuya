package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"google.golang.org/genai"
)

type memRecorder struct{ steps []Step }

func (m *memRecorder) Record(_ context.Context, s Step) { m.steps = append(m.steps, s) }

// A tool round leaves, in order: the model asking for the tool, the call with
// its arguments and result, and the final structured answer.
func TestRunModelRecordsSteps(t *testing.T) {
	rec := &memRecorder{}
	ctx := WithRecorder(context.Background(), rec)
	round := 0
	model := func(ctx context.Context, name string, history []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
		round++
		switch round {
		case 1:
			r := response(&genai.Part{FunctionCall: &genai.FunctionCall{ID: "c1", Name: "getSleepTime", Args: map[string]any{"from": "2026-09-01"}}})
			r.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 10, TotalTokenCount: 12}
			return r, nil
		case 2:
			return response(&genai.Part{Text: "enough"}), nil
		}
		return response(&genai.Part{Text: `{"send":false}`}), nil
	}
	if _, err := RunModel(ctx, model, &fakeTools{}, "job", "m", genai.Text("task"), &genai.GenerateContentConfig{}); err != nil {
		t.Fatal(err)
	}
	kinds := []string{}
	for _, s := range rec.steps {
		kinds = append(kinds, s.Kind+":"+s.Name)
	}
	want := "model:m tool:getSleepTime model:m model:m"
	if strings.Join(kinds, " ") != want {
		t.Fatalf("steps = %v, want %s", kinds, want)
	}
	first := rec.steps[0].Output.(map[string]any)
	if first["round"] != 1 || first["tool_calls"].([]string)[0] != "getSleepTime" || first["tokens"] == nil {
		t.Errorf("model step = %v", first)
	}
	tool := rec.steps[1]
	if !tool.OK || tool.Input.(map[string]any)["from"] != "2026-09-01" || tool.Output.(map[string]any)["output"] == nil {
		t.Errorf("tool step = %+v", tool)
	}
	if last := rec.steps[3].Output.(map[string]any); last["final"] != true || last["text"] != `{"send":false}` {
		t.Errorf("final step = %v", last)
	}
}

// Without a recorder in the context the loop runs as before.
func TestRunModelWithoutRecorder(t *testing.T) {
	model := func(ctx context.Context, name string, history []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
		return response(&genai.Part{Text: `{"send":false}`}), nil
	}
	if _, err := RunModel(context.Background(), model, &fakeTools{}, "job", "m", genai.Text("task"), &genai.GenerateContentConfig{}); err != nil {
		t.Fatal(err)
	}
}

func TestStepJSONTruncates(t *testing.T) {
	if stepJSON(nil) != nil {
		t.Error("nil must stay NULL")
	}
	small := stepJSON(map[string]any{"a": 1})
	if string(small) != `{"a":1}` {
		t.Errorf("small = %s", small)
	}
	big := stepJSON(map[string]any{"s": strings.Repeat("睡", maxStepBytes)})
	var out map[string]any
	if err := json.Unmarshal(big, &out); err != nil {
		t.Fatalf("truncated step is not JSON: %v", err)
	}
	if out["truncated"] != true || len(big) > maxStepBytes+256 {
		t.Errorf("big step not truncated: %d bytes", len(big))
	}
}
