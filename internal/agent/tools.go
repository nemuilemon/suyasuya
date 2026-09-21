// Read-only tools are bound to the owner of one claimed task.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/genai"
	"suyasuya/internal/domain"
)

type ToolData interface {
	AgentNotificationHistory(context.Context, string) ([]json.RawMessage, error)
	UserTimezone(context.Context, string) (string, error)
	AgentSleepSamples(context.Context, string, string, string) ([]domain.SleepSample, error)
	AgentPVT(context.Context, string, string, string, string) ([]json.RawMessage, error)
	AgentSettings(context.Context, string) (json.RawMessage, error)
}
type ToolRegistry struct {
	allowed []string
	data    ToolData
	task    domain.Task
	now     time.Time
}

func NewToolRegistry(data ToolData, task domain.Task) *ToolRegistry {
	return &ToolRegistry{data: data, task: task, now: time.Now()}
}
func (r *ToolRegistry) Declarations() []*genai.FunctionDeclaration {
	dates := &genai.Schema{Type: genai.TypeObject, Properties: map[string]*genai.Schema{
		"from": {Type: genai.TypeString, Description: "開始日 YYYY-MM-DD。from/to の両方を省略すると直近7日。ユーザーの timezone。"},
		"to":   {Type: genai.TypeString, Description: "終了日 YYYY-MM-DD（含む）。最大31日。"},
	}}
	declarations := []*genai.FunctionDeclaration{
		{Name: "getSleepTime", Description: "本人の承認済み主睡眠・昼寝の時間、就寝時刻、日別集計を取得。欠測はnull。週次レポートでは生成時の対象週snapshotだけを参照。", Parameters: dates},
		{Name: "getPVTResults", Description: "本人の反応時間テストの結果を取得。completedとprotocol_versionを区別し、異なる測定方式を混同しない。", Parameters: dates},
		{Name: "getNotificationHistory", Description: "本人への直近20件の通知本文・送信状態・時刻を取得。繰り返しや以前の反応の確認に使う。返信を観測したものではない。", Parameters: &genai.Schema{Type: genai.TypeObject, Properties: map[string]*genai.Schema{}}},
		{Name: "getUserSettings", Description: "本人のtimezone、現在の現地日付、目標就寝時刻、通知時刻を取得。認証情報は含まない。", Parameters: &genai.Schema{Type: genai.TypeObject, Properties: map[string]*genai.Schema{}}},
	}
	if r.allowed == nil {
		return declarations
	}
	filtered := []*genai.FunctionDeclaration{}
	for _, d := range declarations {
		if r.isAllowed(d.Name) {
			filtered = append(filtered, d)
		}
	}
	return filtered

}
func (r *ToolRegistry) Call(ctx context.Context, name string, args map[string]any) map[string]any {
	fail := func(s string) map[string]any { return map[string]any{"error": s} }
	if ctx.Err() != nil {
		return fail("cancelled")
	}
	if !r.isAllowed(name) || (name != "getSleepTime" && name != "getPVTResults" && name != "getUserSettings" && name != "getNotificationHistory") {
		return fail("unknown_tool")
	}
	for key := range args {
		if name == "getUserSettings" || name == "getNotificationHistory" || (key != "from" && key != "to") {
			return fail("unexpected_argument: " + key)
		}
	}
	tz, err := r.data.UserTimezone(ctx, r.task.UserID)
	if err != nil {
		return fail("data_unavailable")
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return fail("timezone_unavailable")
	}
	if name == "getNotificationHistory" {
		history, err := r.data.AgentNotificationHistory(ctx, r.task.UserID)
		if err != nil {
			return fail("data_unavailable")
		}
		return map[string]any{"output": history}
	}
	if name == "getUserSettings" {
		settings, err := r.data.AgentSettings(ctx, r.task.UserID)
		if err != nil {
			return fail("data_unavailable")
		}
		return map[string]any{"output": settings, "local_date": r.now.In(loc).Format("2006-01-02")}
	}
	var snapshot *domain.SleepAnalytics
	if name == "getSleepTime" && r.task.Kind == "weekly_report" {
		var in domain.WeeklyInput
		if json.Unmarshal(r.task.Input, &in) != nil {
			return fail("invalid_task_snapshot")
		}
		snapshot = &in.Report
		tz = snapshot.Timezone
		loc, err = time.LoadLocation(tz)
		if err != nil {
			return fail("invalid_task_snapshot")
		}
	}
	from, to, err := dateRange(args, r.now.In(loc), snapshot)
	if err != nil {
		return fail(err.Error())
	}
	if name == "getPVTResults" {
		sessions, err := r.data.AgentPVT(ctx, r.task.UserID, from, to, tz)
		if err != nil {
			return fail("data_unavailable")
		}
		truncated := len(sessions) > 100
		if truncated {
			sessions = sessions[:100]
		}
		return map[string]any{"output": sessions, "from": from, "to": to, "timezone": tz, "truncated": truncated}
	}
	var samples []domain.SleepSample
	through := r.now.In(loc).Format("2006-01-02")
	source := "current_approved_records"
	if snapshot != nil {
		if from < snapshot.Start || to > snapshot.End {
			return fail("range_outside_report_snapshot")
		}
		samples = snapshot.Samples
		through = snapshot.Through
		source = "report_snapshot"
	} else {
		samples, err = r.data.AgentSleepSamples(ctx, r.task.UserID, from, to)
		if err != nil {
			return fail("data_unavailable_or_range_too_large")
		}
	}
	summary := domain.SummarizeSleep(from, to, through, tz, samples)
	summary.Samples = nil
	return map[string]any{"output": summary, "source": source, "bedtime_units": "minutes since local midnight; values >=1440 mean after midnight on the same sleep night; null means missing"}
}
func dateRange(args map[string]any, now time.Time, snapshot *domain.SleepAnalytics) (string, string, error) {
	if len(args) == 0 {
		if snapshot != nil {
			return snapshot.Start, snapshot.End, nil
		}
		return now.AddDate(0, 0, -6).Format("2006-01-02"), now.Format("2006-01-02"), nil
	}
	from, ok1 := args["from"].(string)
	to, ok2 := args["to"].(string)
	start, e1 := time.Parse("2006-01-02", from)
	end, e2 := time.Parse("2006-01-02", to)
	if !ok1 || !ok2 || e1 != nil || e2 != nil || end.Before(start) || end.Sub(start) > 30*24*time.Hour {
		return "", "", fmt.Errorf("provide from and to as YYYY-MM-DD, at most 31 days")
	}
	return from, to, nil
}

func (r *ToolRegistry) isAllowed(name string) bool {
	if r.allowed == nil {
		return true
	}
	for _, allowed := range r.allowed {
		if name == allowed {
			return true
		}
	}
	return false
}
