package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestQueryReadyBlockedFilters(t *testing.T) {
	planPath := filepath.Join(t.TempDir(), "PLAN.md")
	content := []byte("# Query Fixture\n\n- [ ] Task A\n  @id fixture.query.a\n  @horizon now\n\n- [ ] Task B\n  @id fixture.query.b\n  @horizon next\n  @deps fixture.query.a\n\n- [ ] Task C\n  @id fixture.query.c\n  @horizon later\n")
	if err := os.WriteFile(planPath, content, 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "ready",
			args: []string{"query", "--plan", planPath, "--format", "json", "--ready"},
			want: []string{"fixture.query.a", "fixture.query.c"},
		},
		{
			name: "blocked",
			args: []string{"query", "--plan", planPath, "--format", "json", "--blocked"},
			want: []string{"fixture.query.b"},
		},
		{
			name: "horizon next blocked",
			args: []string{"query", "--plan", planPath, "--format", "json", "--horizon", "next", "--blocked"},
			want: []string{"fixture.query.b"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			var errOut bytes.Buffer
			exit := Run(tc.args, &out, &errOut)
			if exit != 0 {
				t.Fatalf("expected success, got exit=%d stderr=%q", exit, errOut.String())
			}

			var payload map[string]any
			if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
				t.Fatalf("decode query json output: %v", err)
			}
			if payload["command"] != "query" {
				t.Fatalf("expected command query, got %v", payload["command"])
			}
			data, ok := payload["data"].(map[string]any)
			if !ok {
				t.Fatalf("expected data object, got %#v", payload["data"])
			}
			rawTasks, ok := data["tasks"].([]any)
			if !ok {
				t.Fatalf("expected tasks array, got %#v", data["tasks"])
			}
			got := make([]string, 0, len(rawTasks))
			for _, item := range rawTasks {
				taskMap, ok := item.(map[string]any)
				if !ok {
					t.Fatalf("expected task object, got %#v", item)
				}
				id, _ := taskMap["id"].(string)
				got = append(got, id)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("task filter mismatch: want=%v got=%v", tc.want, got)
			}
		})
	}
}

func TestQueryTextUsesPlaceholderForMissingHorizon(t *testing.T) {
	planPath := filepath.Join(t.TempDir(), "PLAN.md")
	content := []byte("# Query Fixture\n\n- [ ] Task A\n  @id fixture.query.no_horizon\n")
	if err := os.WriteFile(planPath, content, 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"query", "--plan", planPath, "--format", "text"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected success, got exit=%d stderr=%q", exit, errOut.String())
	}
	if !strings.Contains(out.String(), "fixture.query.no_horizon [-]") {
		t.Fatalf("expected missing-horizon placeholder in output, got: %q", out.String())
	}
}
