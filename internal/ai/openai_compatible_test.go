package ai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAICompatibleProviderSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if !strings.Contains(string(body), `"model":"gpt-test"`) {
			t.Fatalf("expected model in request body, got %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gpt-test","choices":[{"message":{"role":"assistant","content":"PATCH"}}]}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		BaseURL: server.URL,
		APIKey:  "secret-token",
		Model:   "gpt-test",
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	resp, err := provider.GenerateApplyFix(context.Background(), ApplyFixRequest{
		PlanPath: "PLAN.md",
		PlanHash: "abc123",
		Prompt:   "repair this",
	})
	if err != nil {
		t.Fatalf("generate apply fix: %v", err)
	}
	if resp.Provider != ProviderOpenAICompatible {
		t.Fatalf("unexpected provider: %q", resp.Provider)
	}
	if resp.ProposalText != "PATCH" {
		t.Fatalf("unexpected proposal text: %q", resp.ProposalText)
	}
	if strings.TrimSpace(resp.PromptHash) == "" {
		t.Fatalf("expected non-empty prompt hash")
	}
}

func TestOpenAICompatibleProviderHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		BaseURL: server.URL,
		Model:   "gpt-test",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	_, err = provider.GenerateApplyFix(context.Background(), ApplyFixRequest{Prompt: "x"})
	if err == nil {
		t.Fatalf("expected non-200 error")
	}
	if !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("expected status in error, got %v", err)
	}
}

func TestOpenAICompatibleProviderMissingChoiceContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gpt-test","choices":[]}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		BaseURL: server.URL,
		Model:   "gpt-test",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	_, err = provider.GenerateApplyFix(context.Background(), ApplyFixRequest{Prompt: "x"})
	if err == nil {
		t.Fatalf("expected missing-choice error")
	}
	if !strings.Contains(err.Error(), "missing message content") {
		t.Fatalf("expected missing-content error, got %v", err)
	}
}
