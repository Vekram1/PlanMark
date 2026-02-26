package diag

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

type SourceSpan struct {
	Path      string `json:"path,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

type Diagnostic struct {
	Severity    Severity   `json:"severity"`
	Code        Code       `json:"code"`
	Message     string     `json:"message"`
	Source      SourceSpan `json:"source,omitempty"`
	Fingerprint string     `json:"fingerprint,omitempty"`
}
