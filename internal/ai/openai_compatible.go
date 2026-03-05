package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAICompatibleConfig struct {
	BaseURL    string
	APIKey     string
	Model      string
	Timeout    time.Duration
	HTTPClient *http.Client
}

func NewOpenAICompatibleProvider(cfg OpenAICompatibleConfig) (Provider, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("%w: missing base_url", ErrProviderNotConfigured)
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return nil, fmt.Errorf("%w: missing model", ErrProviderNotConfigured)
	}
	client := cfg.HTTPClient
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	return &openAICompatibleProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  strings.TrimSpace(cfg.APIKey),
		model:   model,
		client:  client,
	}, nil
}

type openAICompatibleProvider struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

type openAIChatRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
}

func (p *openAICompatibleProvider) GenerateApplyFix(ctx context.Context, req ApplyFixRequest) (ApplyFixResponse, error) {
	body := openAIChatRequest{
		Model: p.model,
		Messages: []openAIMessage{
			{
				Role:    "system",
				Content: "Return a deterministic, reviewable PLAN.md patch or Plan Delta proposal only.",
			},
			{
				Role:    "user",
				Content: strings.TrimSpace(req.Prompt),
			},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return ApplyFixResponse{}, fmt.Errorf("marshal openai request: %w", err)
	}

	endpoint := p.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return ApplyFixResponse{}, fmt.Errorf("build openai request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return ApplyFixResponse{}, fmt.Errorf("openai request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return ApplyFixResponse{}, fmt.Errorf("read openai response: %w", err)
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return ApplyFixResponse{}, fmt.Errorf("openai response status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed openAIChatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return ApplyFixResponse{}, fmt.Errorf("decode openai response: %w", err)
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return ApplyFixResponse{}, fmt.Errorf("openai response missing message content")
	}

	model := strings.TrimSpace(parsed.Model)
	if model == "" {
		model = p.model
	}

	return ApplyFixResponse{
		Provider:     ProviderOpenAICompatible,
		Model:        model,
		ProposalType: "plan_delta_preview",
		ProposalText: strings.TrimSpace(parsed.Choices[0].Message.Content),
		PromptHash:   PromptSHA256(req.Prompt),
	}, nil
}
