package diag

type Code string

const (
	CodeUnattachedMetadata   Code = "UNATTACHED_METADATA"
	CodePathTraversalReject  Code = "PATH_TRAVERSAL_REJECTED"
	CodeDuplicateTaskID      Code = "DUPLICATE_TASK_ID"
	CodeUnknownDependency    Code = "UNKNOWN_DEPENDENCY"
	CodeDependencyCycle      Code = "DEPENDENCY_CYCLE"
	CodeMissingAccept        Code = "MISSING_ACCEPT"
	CodeUnknownHorizon       Code = "UNKNOWN_HORIZON"
	CodeCompileLimitExceeded Code = "COMPILE_LIMIT_EXCEEDED"
)
