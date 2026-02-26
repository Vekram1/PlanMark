package policy

import "testing"

func TestPolicyRegistrySelectsExplicitVersion(t *testing.T) {
	reg := NewRegistry()
	got, err := reg.Select(KindDeterminism, "v0.1")
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if got != "v0.1" {
		t.Fatalf("expected v0.1, got %q", got)
	}
}
