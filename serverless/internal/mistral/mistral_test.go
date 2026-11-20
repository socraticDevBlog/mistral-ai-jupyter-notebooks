package mistral

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompleteSendsKeyAndParsesReply(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello there"}}]}`))
	}))
	defer srv.Close()

	c := &Client{APIKey: "secret-key", Model: "test-model", BaseURL: srv.URL, HTTP: srv.Client()}
	got, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, 128)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got != "hello there" {
		t.Errorf("got %q, want %q", got, "hello there")
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("Authorization header = %q", gotAuth)
	}
	var req chatRequest
	if err := json.Unmarshal([]byte(gotBody), &req); err != nil {
		t.Fatalf("request body not valid JSON: %v", err)
	}
	if req.Model != "test-model" || req.MaxTokens != 128 || len(req.Messages) != 1 {
		t.Errorf("unexpected request body: %+v", req)
	}
}

func TestCompleteAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid api key"}`))
	}))
	defer srv.Close()

	c := &Client{APIKey: "bad", Model: "m", BaseURL: srv.URL, HTTP: srv.Client()}
	_, err := c.Complete(context.Background(), nil, 10)
	if err == nil {
		t.Fatal("expected an error for a 401 response")
	}
}

func TestNewFromEnvRequiresKey(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "")
	if _, err := NewFromEnv(); err == nil {
		t.Fatal("expected error when MISTRAL_API_KEY is unset")
	}
	t.Setenv("MISTRAL_API_KEY", "k")
	c, err := NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}
	if c.Model != DefaultModel || c.BaseURL != DefaultBaseURL {
		t.Errorf("defaults not applied: %+v", c)
	}
}
