// Package mistral is a minimal client for the Mistral AI chat-completions API.
//
// It deliberately uses plain net/http (no SDK) so the Lambda binary stays
// small and the code easy to read. The API key lives only in the Lambda's
// environment and never leaves the server — web users cannot see or steal it.
package mistral

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	// DefaultBaseURL is the Mistral API v1 endpoint.
	DefaultBaseURL = "https://api.mistral.ai/v1"
	// DefaultModel mirrors DEFAULT_MODEL in mistral_jupyter/mistral_client.py.
	DefaultModel = "mistral-small-latest"
	// httpTimeout bounds ONE upstream call. A dialogue round makes two
	// sequential calls, and API Gateway REST APIs hard-cap at 29s total
	// integration time, so 2x12s worst case fits inside the gateway limit.
	httpTimeout = 12 * time.Second
)

// Message is one chat message (role: system | user | assistant).
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LLM is the narrow interface the notebook handlers depend on.
// Handlers take an LLM, so tests can inject a fake — no API key needed.
type LLM interface {
	Complete(ctx context.Context, messages []Message, maxTokens int) (string, error)
}

// Client calls the Mistral chat-completions endpoint.
type Client struct {
	APIKey  string
	Model   string
	BaseURL string
	HTTP    *http.Client
}

// NewFromEnv builds a Client from environment variables:
//
//	MISTRAL_API_KEY   (required) never send this to the browser
//	MISTRAL_MODEL     (optional) defaults to mistral-small-latest
//	MISTRAL_BASE_URL  (optional) defaults to https://api.mistral.ai/v1 —
//	                  override in tests to point at a fake server
func NewFromEnv() (*Client, error) {
	key := os.Getenv("MISTRAL_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("MISTRAL_API_KEY is not set")
	}
	model := os.Getenv("MISTRAL_MODEL")
	if model == "" {
		model = DefaultModel
	}
	base := os.Getenv("MISTRAL_BASE_URL")
	if base == "" {
		base = DefaultBaseURL
	}
	return &Client{
		APIKey:  key,
		Model:   model,
		BaseURL: base,
		HTTP:    &http.Client{Timeout: httpTimeout},
	}, nil
}

// wire formats for POST {BaseURL}/chat/completions

type chatRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	MaxTokens int       `json:"max_tokens"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

// Complete runs one chat completion and returns the assistant's text.
func (c *Client) Complete(ctx context.Context, messages []Message, maxTokens int) (string, error) {
	body, err := json.Marshal(chatRequest{Model: c.Model, Messages: messages, MaxTokens: maxTokens})
	if err != nil {
		return "", fmt.Errorf("encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling mistral: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		return "", fmt.Errorf("reading mistral response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("mistral API error (status %d): %s", resp.StatusCode, string(data))
	}

	var out chatResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("decoding mistral response: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("mistral returned no choices")
	}
	return out.Choices[0].Message.Content, nil
}
