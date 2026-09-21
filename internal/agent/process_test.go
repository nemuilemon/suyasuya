package agent

import (
	"context"
	"encoding/json"
	"suyasuya/internal/domain"
	"testing"
)

func TestDispatchTask(t *testing.T) {
	task := domain.Task{ID: "job", UserID: "owner", Kind: "custom_task", BasePrompt: "shared base", Input: json.RawMessage(`{"value":1}`)}
	runner := &Runner{handlers: map[string]Handler{"custom_task": func(ctx context.Context, got domain.Task) ([]byte, error) {
		if got.BasePrompt != "shared base" || got.Instructions("specific") != "shared base\n\nspecific" || got.UserID != task.UserID || string(got.Input) != string(task.Input) {
			t.Fatal("task context lost")
		}
		return []byte(`{"ok":true}`), nil
	}}}
	output, err := runner.execute(context.Background(), task)
	if err != nil || string(output) != `{"ok":true}` {
		t.Fatalf("dispatch: %s %v", output, err)
	}
	task.Kind = "unknown"
	if _, err = runner.execute(context.Background(), task); err == nil {
		t.Fatal("unregistered task must fail")
	}
}
