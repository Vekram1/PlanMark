package cache

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func FuzzContextPacketKeyDeterminism(f *testing.F) {
	f.Add("L0", "PLAN.md", "v0.2", "v0.1", "v0.1", "task.a", "node.a", "fp.a", "hash.a", "c\na\nb")
	f.Add("", "", "", "", "", "", "", "", "", "")

	f.Fuzz(func(t *testing.T, level string, planPath string, irVersion string, determinismVersion string, semanticVersion string, taskID string, nodeRef string, taskFP string, sliceHash string, pinHashesRaw string) {
		input := ContextKeyInput{
			Level:                           level,
			PlanPath:                        planPath,
			IRVersion:                       irVersion,
			DeterminismPolicyVersion:        determinismVersion,
			SemanticDerivationPolicyVersion: semanticVersion,
			TaskID:                          taskID,
			TaskNodeRef:                     nodeRef,
			TaskSemanticFingerprint:         taskFP,
			NodeSliceHash:                   sliceHash,
			PinTargetHashes:                 splitCacheLines(pinHashesRaw),
		}

		first := ContextPacketKey(input)
		second := ContextPacketKey(input)
		if first != second {
			t.Fatalf("nondeterministic context packet key: %q vs %q", first, second)
		}
	})
}

func FuzzCompileReuseKeyDeterminism(f *testing.F) {
	f.Add("PLAN.md", "content-hash", "parser:v0.1", "v0.2", "v0.1", "v0.1", "cfg-hash")
	f.Add("", "", "", "", "", "", "")

	f.Fuzz(func(t *testing.T, planPath string, planHash string, parserFP string, irVersion string, determinismVersion string, semanticVersion string, configHash string) {
		input := CompileReuseInput{
			PlanPath:                        planPath,
			PlanContentHash:                 planHash,
			ParserFingerprint:               parserFP,
			IRVersion:                       irVersion,
			DeterminismPolicyVersion:        determinismVersion,
			SemanticDerivationPolicyVersion: semanticVersion,
			EffectiveConfigHash:             configHash,
		}
		first := CompileReuseKey(input)
		second := CompileReuseKey(input)
		if first != second {
			t.Fatalf("nondeterministic compile reuse key: %q vs %q", first, second)
		}
	})
}

func FuzzContextPacketPathValidation(f *testing.F) {
	f.Add(".planmark", strings.Repeat("a", 64))
	f.Add("", "bad-key")

	f.Fuzz(func(t *testing.T, stateDir string, key string) {
		pathA, errA := contextPacketPath(stateDir, key)
		pathB, errB := contextPacketPath(stateDir, key)
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic cache path error state: %v vs %v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic cache path error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if pathA != pathB {
			t.Fatalf("nondeterministic cache path: %q vs %q", pathA, pathB)
		}
	})
}

func FuzzWriteReadContextPacketRoundTrip(f *testing.F) {
	f.Add(strings.Repeat("a", 64), []byte("{\"level\":\"L0\"}\n"))
	f.Add(strings.Repeat("f", 64), []byte(""))

	f.Fuzz(func(t *testing.T, key string, payload []byte) {
		if len(key) != 64 || strings.Trim(key, "abcdef0123456789") != "" {
			return
		}
		tmp := t.TempDir()

		pathA, errA := WriteContextPacket(tmp, key, payload)
		pathB, errB := WriteContextPacket(tmp, key, payload)
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic write error state: %v vs %v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic write error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if pathA != pathB {
			t.Fatalf("nondeterministic cache write path: %q vs %q", pathA, pathB)
		}

		gotA, errA := ReadContextPacket(tmp, key)
		gotB, errB := ReadContextPacket(tmp, key)
		if (errA != nil) != (errB != nil) {
			t.Fatalf("nondeterministic read error state: %v vs %v", errA, errB)
		}
		if errA != nil {
			if errA.Error() != errB.Error() {
				t.Fatalf("nondeterministic read error text: %q vs %q", errA.Error(), errB.Error())
			}
			return
		}
		if !bytes.Equal(gotA, gotB) || !bytes.Equal(gotA, payload) {
			t.Fatalf("nondeterministic cache payload round trip")
		}
	})
}

func FuzzSanitizeLockNameDeterminism(f *testing.F) {
	f.Add("context-packet")
	f.Add(" ../ weird\tname ### ")

	f.Fuzz(func(t *testing.T, raw string) {
		first := sanitizeLockName(raw)
		second := sanitizeLockName(raw)
		if first != second {
			t.Fatalf("nondeterministic lock name: %q vs %q", first, second)
		}
		if first == "" {
			t.Fatalf("empty sanitized lock name for %q", raw)
		}
	})
}

func FuzzAcquireReleaseLockDeterminism(f *testing.F) {
	f.Add("context-packet")
	f.Add(" ../ weird\tname ### ")

	f.Fuzz(func(t *testing.T, rawName string) {
		tmp := t.TempDir()

		lock, err := AcquireLock(tmp, rawName)
		if err != nil {
			t.Fatalf("first acquire lock: %v", err)
		}
		second, err := AcquireLock(tmp, rawName)
		if !errors.Is(err, ErrLockHeld) {
			if second != nil {
				_ = second.Release()
			}
			t.Fatalf("expected ErrLockHeld on second acquire, got lock=%v err=%v", second, err)
		}

		if err := lock.Release(); err != nil {
			t.Fatalf("release lock: %v", err)
		}
		if err := lock.Release(); err != nil {
			t.Fatalf("second release should be harmless, got %v", err)
		}

		reacquired, err := AcquireLock(tmp, rawName)
		if err != nil {
			t.Fatalf("reacquire lock: %v", err)
		}
		if err := reacquired.Release(); err != nil {
			t.Fatalf("release reacquired lock: %v", err)
		}
	})
}

func splitCacheLines(raw string) []string {
	parts := strings.Split(raw, "\n")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
