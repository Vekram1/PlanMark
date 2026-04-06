package protocol

import (
	"bytes"
	"encoding/json"
	"testing"
)

type fuzzData struct {
	Name   string `json:"name"`
	Body   string `json:"body,omitempty"`
	Active bool   `json:"active"`
}

func FuzzEnvelopeJSONDeterminism(f *testing.F) {
	f.Add("v0.1", "compile", "ok", "planmark", "task", "body", true)
	f.Add("", "query", "error", "", "", "", false)

	f.Fuzz(func(t *testing.T, schema string, command string, status string, tool string, name string, body string, active bool) {
		env := Envelope[fuzzData]{
			SchemaVersion: schema,
			ToolVersion:   tool,
			Command:       command,
			Status:        status,
			Data: fuzzData{
				Name:   name,
				Body:   body,
				Active: active,
			},
		}

		jsonA, err := json.Marshal(env)
		if err != nil {
			t.Fatalf("marshal envelope A: %v", err)
		}
		jsonB, err := json.Marshal(env)
		if err != nil {
			t.Fatalf("marshal envelope B: %v", err)
		}
		if !bytes.Equal(jsonA, jsonB) {
			t.Fatalf("nondeterministic envelope json")
		}

		var decoded map[string]any
		if err := json.Unmarshal(jsonA, &decoded); err != nil {
			t.Fatalf("unmarshal envelope json: %v", err)
		}
		if _, ok := decoded["schema_version"]; !ok {
			t.Fatalf("missing schema_version field in envelope json")
		}
		if _, ok := decoded["command"]; !ok {
			t.Fatalf("missing command field in envelope json")
		}
		if _, ok := decoded["status"]; !ok {
			t.Fatalf("missing status field in envelope json")
		}
		if _, ok := decoded["data"]; !ok {
			t.Fatalf("missing data field in envelope json")
		}
	})
}
