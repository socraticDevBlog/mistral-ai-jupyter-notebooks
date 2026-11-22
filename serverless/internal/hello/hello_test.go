package hello

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/socraticDevBlog/mistral-ai-jupyter-notebooks/serverless/internal/lambdaapi"
	"github.com/socraticDevBlog/mistral-ai-jupyter-notebooks/serverless/internal/mistral"
)

type fakeLLM struct{ got []mistral.Message }

func (f *fakeLLM) Complete(ctx context.Context, msgs []mistral.Message, maxTokens int) (string, error) {
	f.got = msgs
	return "a proof-of-concept is a small experiment.", nil
}

func TestHandlerAnswersPrompt(t *testing.T) {
	llm := &fakeLLM{}
	out, err := Handler(llm)(context.Background(), &lambdaapi.Request{
		Query: map[string][]string{"prompt": {"What is a proof-of-concept?"}},
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	resp := out.(Response)
	if resp.Answer == "" {
		t.Error("empty answer")
	}
	if llm.got[0].Role != "system" || llm.got[1].Content != "What is a proof-of-concept?" {
		t.Errorf("messages wrong: %+v", llm.got)
	}
}

func TestHandlerRejectsBadInput(t *testing.T) {
	h := Handler(&fakeLLM{})
	if _, err := h(context.Background(), &lambdaapi.Request{Query: nil}); err == nil {
		t.Error("expected error for missing prompt")
	}
	long := strings.Repeat("x", maxPromptChars+1)
	_, err := h(context.Background(), &lambdaapi.Request{Query: map[string][]string{"prompt": {long}}})
	var ce *lambdaapi.ClientError
	if !errors.As(err, &ce) {
		t.Errorf("expected ClientError for over-long prompt, got %v", err)
	}
}
