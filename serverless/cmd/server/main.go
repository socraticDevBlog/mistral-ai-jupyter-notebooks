// Command server is a single AWS Lambda function that exposes the notebooks
// in this repository as GET-only REST endpoints behind API Gateway.
//
// To add another notebook: write a small package under internal/ that exposes
// a Handler(llm) lambdaapi.HandlerFunc, then register one route for it below.
package main

import (
	"log"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/socraticDevBlog/mistral-ai-jupyter-notebooks/serverless/internal/dialogue"
	"github.com/socraticDevBlog/mistral-ai-jupyter-notebooks/serverless/internal/hello"
	"github.com/socraticDevBlog/mistral-ai-jupyter-notebooks/serverless/internal/lambdaapi"
	"github.com/socraticDevBlog/mistral-ai-jupyter-notebooks/serverless/internal/mistral"
)

func main() {
	// The API key is read from the Lambda environment only — it is never
	// sent to the browser, so web users can neither abuse nor steal it.
	llm, err := mistral.NewFromEnv()
	if err != nil {
		log.Fatalf("startup: %v", err)
	}

	router := lambdaapi.NewRouter()
	router.Add("/dialogue", dialogue.Handler(llm)) // notebooks/01_dialogue.ipynb
	router.Add("/hello", hello.Handler(llm))       // notebooks/00_hello_mistral.ipynb

	lambda.Start(router.Handle)
}
