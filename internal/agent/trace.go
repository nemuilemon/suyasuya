// Trace records what one task run did: each model round, each tool call with
// its arguments and result, and how the run ended. It is for the owner to read
// back, not for the agent to reason over.
package agent

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"
	"unicode/utf8"

	"suyasuya/internal/platform"
)

// Step kinds, in the order a run produces them.
const (
	StepStart = "start"
	StepModel = "model"
	StepTool  = "tool"
	StepFinal = "final"
	StepError = "error"
)

// maxStepBytes caps what one step stores per field. Tool results can carry a
// month of sleep records; past this the step keeps a preview instead.
const maxStepBytes = 8 << 10

type Step struct {
	Kind     string
	Name     string
	Input    any
	Output   any
	OK       bool
	Duration time.Duration
}

type Recorder interface {
	Record(context.Context, Step)
}

type nopRecorder struct{}

func (nopRecorder) Record(context.Context, Step) {}

type recorderKey struct{}

// WithRecorder attaches the run's recorder, so handlers keep their signature
// and RunModel finds it without every caller passing it through.
func WithRecorder(ctx context.Context, r Recorder) context.Context {
	return context.WithValue(ctx, recorderKey{}, r)
}

func recorderFrom(ctx context.Context) Recorder {
	if r, ok := ctx.Value(recorderKey{}).(Recorder); ok {
		return r
	}
	return nopRecorder{}
}

// StepStore is the write side the recorder needs. The insert is guarded by the
// lease, so a run that lost its lease cannot add to the trace.
type StepStore interface {
	InsertStep(ctx context.Context, jobID, userID, lease string, seq int, kind, name string, input, output []byte, ok bool, durationMS int64) error
}

type dbRecorder struct {
	store                StepStore
	jobID, userID, lease string

	mu  sync.Mutex
	seq int
}

func newDBRecorder(store StepStore, jobID, userID, lease string) *dbRecorder {
	return &dbRecorder{store: store, jobID: jobID, userID: userID, lease: lease}
}

// Record never fails the task: a trace that could not be written is logged and
// the run goes on. The write uses its own short context so the final steps of
// a cancelled or timed-out run still land.
func (r *dbRecorder) Record(_ context.Context, s Step) {
	r.mu.Lock()
	r.seq++
	seq := r.seq
	r.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := r.store.InsertStep(ctx, r.jobID, r.userID, r.lease, seq, s.Kind, s.Name,
		stepJSON(s.Input), stepJSON(s.Output), s.OK, s.Duration.Milliseconds())
	if err != nil {
		log.Printf("task %s step %d: %s", r.jobID, seq, platform.SafeError(err))
	}
}

// stepJSON encodes v for a jsonb column: nil stays NULL, and anything larger
// than maxStepBytes becomes {"truncated":true,"bytes":n,"preview":"…"}.
func stepJSON(v any) []byte {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		b, _ = json.Marshal(map[string]any{"unencodable": err.Error()})
	}
	if len(b) <= maxStepBytes {
		return b
	}
	preview := b[:maxStepBytes]
	for !utf8.Valid(preview) {
		preview = preview[:len(preview)-1]
	}
	out, _ := json.Marshal(map[string]any{"truncated": true, "bytes": len(b), "preview": string(preview)})
	return out
}
