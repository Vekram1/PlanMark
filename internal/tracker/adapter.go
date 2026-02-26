package tracker

import "context"

// TaskProjection is the canonical task payload projected from PLAN/IR.
type TaskProjection struct {
	ID                string
	Title             string
	Anchor            string
	NodeRef           string
	SourcePath        string
	SourceStartLine   int
	SourceEndLine     int
	SourceHash        string
	CompileID         string
	Accept            []string
	ProjectionVersion string
}

type TaskProvenance struct {
	NodeRef    string
	Path       string
	StartLine  int
	EndLine    int
	SourceHash string
	CompileID  string
}

// RuntimeFields are the tracker-owned mutable overlays pulled safely from remote state.
type RuntimeFields struct {
	Status   string
	Assignee string
	Priority string
}

// PushResult captures idempotent push behavior.
type PushResult struct {
	RemoteID   string
	Mutated    bool
	Noop       bool
	Diagnostic string
}

// TrackerAdapter defines the base adapter contract.
// - push_task: push canonical plan-derived task projection to tracker.
// - pull_runtime_fields: pull tracker-owned runtime overlays only.
type TrackerAdapter interface {
	PushTask(ctx context.Context, task TaskProjection) (PushResult, error)
	PullRuntimeFields(ctx context.Context, ids []string) (map[string]RuntimeFields, error)
}
