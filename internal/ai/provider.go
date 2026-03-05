package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	ProviderOpenAICompatible = "openai_compatible"
	ProviderDeterministic    = "deterministic_mock"
)

var (
	ErrProviderNotConfigured = errors.New("ai provider not configured")
	ErrUnsupportedProvider   = errors.New("unsupported ai provider")
)

type ApplyFixRequest struct {
	PlanPath string
	PlanHash string
	Prompt   string
	Model    string
}

type ApplyFixResponse struct {
	Provider     string
	Model        string
	ProposalType string
	ProposalText string
	PromptHash   string
}

type Provider interface {
	GenerateApplyFix(ctx context.Context, req ApplyFixRequest) (ApplyFixResponse, error)
}

func NewProvider(name string) (Provider, error) {
	switch strings.TrimSpace(name) {
	case "":
		return nil, ErrProviderNotConfigured
	case ProviderDeterministic:
		return deterministicMockProvider{}, nil
	case ProviderOpenAICompatible:
		return nil, fmt.Errorf("%w: %s (adapter not wired yet)", ErrUnsupportedProvider, ProviderOpenAICompatible)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProvider, strings.TrimSpace(name))
	}
}

func PromptSHA256(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}

type deterministicMockProvider struct{}

func (deterministicMockProvider) GenerateApplyFix(_ context.Context, req ApplyFixRequest) (ApplyFixResponse, error) {
	promptHash := PromptSHA256(req.Prompt)
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = "deterministic/mock-v1"
	}
	return ApplyFixResponse{
		Provider:     ProviderDeterministic,
		Model:        model,
		ProposalType: "plan_delta_preview",
		PromptHash:   promptHash,
		ProposalText: strings.Join([]string{
			"# Deterministic apply-fix proposal (mock)",
			"- target: " + strings.TrimSpace(req.PlanPath),
			"- base_plan_hash: " + strings.TrimSpace(req.PlanHash),
			"- prompt_hash: " + promptHash,
			"- note: no model call performed; this is deterministic scaffolding.",
		}, "\n"),
	}, nil
}
