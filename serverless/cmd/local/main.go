// Command local runs the exact same routes as the Lambda on your machine,
// so you can curl the endpoints without deploying:
//
//	export MISTRAL_API_KEY=...   # same env var the Lambda uses
//	go run ./cmd/local
//	curl 'http://localhost:8080/hello?prompt=hi'
package main

import (
	"log"
	"net/http"

	"github.com/socraticDevBlog/mistral-ai-jupyter-notebooks/serverless/internal/dialogue"
	"github.com/socraticDevBlog/mistral-ai-jupyter-notebooks/serverless/internal/hello"
	"github.com/socraticDevBlog/mistral-ai-jupyter-notebooks/serverless/internal/lambdaapi"
	"github.com/socraticDevBlog/mistral-ai-jupyter-notebooks/serverless/internal/mistral"
)

func main() {
	llm, err := mistral.NewFromEnv()
	if err != nil {
		log.Fatalf("startup: %v (export MISTRAL_API_KEY first)", err)
	}

	router := lambdaapi.NewRouter()
	router.Add("/dialogue", dialogue.Handler(llm))
	router.Add("/hello", hello.Handler(llm))

	log.Println("listening on http://localhost:8080")
	log.Println("try: curl 'http://localhost:8080/hello?prompt=What%20is%20a%20PoC?'")
	log.Fatal(http.ListenAndServe(":8080", router))
}
