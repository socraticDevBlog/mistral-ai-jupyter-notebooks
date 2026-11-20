// Package hello ports notebooks/00_hello_mistral.ipynb (a one-shot chat
// completion) to GET /hello?prompt=... — the simplest example of how to add
// a notebook to the Lambda: one package, one route in cmd/server.
package hello

import (
	"context"
	"strings"

	"github.com/socraticDevBlog/mistral-ai-jupyter-notebooks/serverless/internal/lambdaapi"
	"github.com/socraticDevBlog/mistral-ai-jupyter-notebooks/serverless/internal/mistral"
)

// maxPromptChars caps the incoming prompt so a web user cannot burn tokens
// with a huge query string.
const maxPromptChars = 1000

// Response is the JSON body returned by GET /hello.
type Response struct {
	Answer string `json:"answer"`
}

// Handler returns the lambdaapi.HandlerFunc for GET /hello.
//
// Query parameters:
//
//	prompt  (required) the user's question
func Handler(llm mistral.LLM) lambdaapi.HandlerFunc {
	return func(ctx context.Context, r *lambdaapi.Request) (any, error) {
		prompt := strings.TrimSpace(r.Query.Get("prompt"))
		if prompt == "" {
			return nil, lambdaapi.BadRequest("missing required query parameter: prompt")
		}
		if n := len([]rune(prompt)); n > maxPromptChars {
			return nil, lambdaapi.BadRequest("prompt too long: %d chars, max %d", n, maxPromptChars)
		}
		answer, err := llm.Complete(ctx, []mistral.Message{
			{Role: "system", Content: "You are a concise assistant."},
			{Role: "user", Content: prompt},
		}, 512)
		if err != nil {
			return nil, err
		}
		return Response{Answer: answer}, nil
	}
}
