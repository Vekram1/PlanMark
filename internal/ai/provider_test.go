package ai

import (
	"context"
	"strings"
	"testing"
)

func TestNewProviderEmptyReturnsNotConfigured(t *testing.T) {
	_, err := NewProvider("")
	if err == nil {
		t.Fatalf("expected not configured error")
	}
	if !strings.Contains(err.Error(), ErrProviderNotConfigured.Error()) {
		t.Fatalf("expected not configured error, got %v", err)
	}
}

func TestNewProviderUnknownReturnsUnsupported(t *testing.T) {
	_, err := NewProvider("made_up_provider")
	if err == nil {
		t.Fatalf("expected unsupported provider error")
	}
	if !strings.Contains(err.Error(), ErrUnsupportedProvider.Error()) {
		t.Fatalf("expected unsupported provider error, got %v", err)
	}
}

func TestDeterministicProviderStableOutput(t *testing.T) {
	provider, err := NewProvider(ProviderDeterministic)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	req := ApplyFixRequest{
		PlanPath: "PLAN.md",
		PlanHash: "abc123",
		Prompt:   "fix missing accept",
		Model:    "",
	}
	gotA, err := provider.GenerateApplyFix(context.Background(), req)
	if err != nil {
		t.Fatalf("generate apply fix A: %v", err)
	}
	gotB, err := provider.GenerateApplyFix(context.Background(), req)
	if err != nil {
		t.Fatalf("generate apply fix B: %v", err)
	}

	if gotA != gotB {
		t.Fatalf("expected deterministic output\na=%+v\nb=%+v", gotA, gotB)
	}
	if gotA.Provider != ProviderDeterministic {
		t.Fatalf("unexpected provider: %q", gotA.Provider)
	}
	if gotA.ProposalType != "plan_delta_preview" {
		t.Fatalf("unexpected proposal type: %q", gotA.ProposalType)
	}
	if !strings.Contains(gotA.ProposalText, "base_plan_hash: abc123") {
		t.Fatalf("expected plan hash in proposal text, got %q", gotA.ProposalText)
	}
}
