package protocol

type Envelope[T any] struct {
	SchemaVersion string `json:"schema_version"`
	ToolVersion   string `json:"tool_version,omitempty"`
	Command       string `json:"command"`
	Status        string `json:"status"`
	Data          T      `json:"data"`
}
