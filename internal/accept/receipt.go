package accept

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	ReceiptSchemaVersion = "v0.1"
	PolicyVersion        = "v0.1"
)

type Digest struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

type CommandRecord struct {
	TaskID        string `json:"task_id"`
	AcceptIndex   int    `json:"accept_index"`
	AcceptText    string `json:"accept_text"`
	CommandText   string `json:"command_text"`
	CommandDigest Digest `json:"command_digest"`
}

type PolicyRecord struct {
	PolicyVersion string `json:"policy_version"`
	CWDPolicy     string `json:"cwd_policy"`
	EnvPolicy     string `json:"env_policy"`
	TimeoutMS     int    `json:"timeout_ms"`
	NetworkPolicy string `json:"network_policy"`
	SandboxPolicy string `json:"sandbox_policy"`
}

type ResultRecord struct {
	ExitStatus int    `json:"exit_status"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	DurationMS int64  `json:"duration_ms"`
	Status     string `json:"status"`
}

type CaptureStream struct {
	Captured  bool    `json:"captured"`
	Bytes     int     `json:"bytes"`
	Digest    *Digest `json:"digest,omitempty"`
	Truncated bool    `json:"truncated"`
}

type RedactionRecord struct {
	Applied bool     `json:"applied"`
	Policy  string   `json:"policy,omitempty"`
	Notes   []string `json:"notes,omitempty"`
}

type CaptureRecord struct {
	Stdout    CaptureStream   `json:"stdout"`
	Stderr    CaptureStream   `json:"stderr"`
	Redaction RedactionRecord `json:"redaction"`
}

type ContextRecord struct {
	RepoRoot string `json:"repo_root,omitempty"`
	PlanPath string `json:"plan_path,omitempty"`
	StateDir string `json:"state_dir,omitempty"`
	Host     string `json:"host,omitempty"`
	RunnerID string `json:"runner_id,omitempty"`
}

type VerificationReceipt struct {
	SchemaVersion string        `json:"schema_version"`
	ToolVersion   string        `json:"tool_version"`
	ReceiptID     string        `json:"receipt_id"`
	CreatedAt     string        `json:"created_at"`
	Command       CommandRecord `json:"command"`
	Policy        PolicyRecord  `json:"policy"`
	Result        ResultRecord  `json:"result"`
	Capture       CaptureRecord `json:"capture"`
	Context       ContextRecord `json:"context,omitempty"`
}

func ParseCommandText(acceptText string) (string, error) {
	trimmed := strings.TrimSpace(acceptText)
	if !strings.HasPrefix(trimmed, "cmd:") {
		return "", fmt.Errorf("unsupported @accept format %q: expected cmd:<command>", acceptText)
	}
	commandText := strings.TrimSpace(strings.TrimPrefix(trimmed, "cmd:"))
	if commandText == "" {
		return "", fmt.Errorf("empty command in @accept line")
	}
	return commandText, nil
}

func NewReceiptID(taskID string, acceptIndex int, acceptText string, startedAt time.Time) string {
	payload := fmt.Sprintf("%s|%d|%s|%s", taskID, acceptIndex, acceptText, startedAt.UTC().Format(time.RFC3339Nano))
	sum := sha256.Sum256([]byte(payload))
	return "rcpt_" + hex.EncodeToString(sum[:16])
}

func NewDigest(text string) Digest {
	sum := sha256.Sum256([]byte(text))
	return Digest{Algorithm: "sha256", Value: hex.EncodeToString(sum[:])}
}

func NewCaptureStream(payload []byte) CaptureStream {
	stream := CaptureStream{
		Captured:  true,
		Bytes:     len(payload),
		Truncated: false,
	}
	if len(payload) > 0 {
		sum := sha256.Sum256(payload)
		stream.Digest = &Digest{Algorithm: "sha256", Value: hex.EncodeToString(sum[:])}
	}
	return stream
}
