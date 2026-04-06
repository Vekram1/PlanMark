package config

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzLoadForPlanDeterminism(f *testing.F) {
	f.Add([]byte("schema_version: v0.1\nprofiles:\n  doctor: exec\ntracker:\n  adapter: beads\n"), true)
	f.Add([]byte("unknown: yes\n"), true)
	f.Add([]byte(""), false)

	f.Fuzz(func(t *testing.T, configBody []byte, createConfig bool) {
		tmp := t.TempDir()
		planPath := filepath.Join(tmp, "PLAN.md")
		if err := os.WriteFile(planPath, []byte("- [ ] Task\n  @id task.one\n"), 0o644); err != nil {
			t.Fatalf("write plan fixture: %v", err)
		}
		if createConfig {
			if err := os.WriteFile(filepath.Join(tmp, ".planmark.yaml"), configBody, 0o644); err != nil {
				t.Fatalf("write config fixture: %v", err)
			}
		}

		first, errA := LoadForPlan(planPath)
		second, errB := LoadForPlan(planPath)
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic config error state: %v vs %v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic config error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if first != second {
			t.Fatalf("nondeterministic resolved config: %#v vs %#v", first, second)
		}
	})
}
