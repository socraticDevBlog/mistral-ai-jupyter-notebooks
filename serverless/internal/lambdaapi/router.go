// Package lambdaapi adapts plain Go handler functions into AWS Lambda
// functions fronted by API Gateway (REST API, proxy integration).
//
// One Lambda binary can serve many notebook endpoints: each notebook gets a
// package exposing a Handler, and cmd/server registers one route per notebook.
// The same Router also implements http.Handler, so you can run and curl the
// exact same code locally without deploying (see cmd/local).
package lambdaapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/aws/aws-lambda-go/events"
)

// Request is the decoded incoming GET request handed to a handler.
type Request struct {
	// Query holds the URL query parameters, e.g. ?message=hi&transcript=...
	Query url.Values
}

// HandlerFunc implements one notebook's logic. The returned value is sent
// back to the caller as a JSON body. Return a ClientError (see BadRequest)
// for a 400 response; any other error becomes a 500.
type HandlerFunc func(ctx context.Context, r *Request) (any, error)

// ClientError marks the request as the caller's fault (HTTP 400).
type ClientError struct{ Msg string }

func (e *ClientError) Error() string { return e.Msg }

// BadRequest builds a 400 error with a formatted message for the web user.
func BadRequest(format string, args ...any) error {
	return &ClientError{Msg: fmt.Sprintf(format, args...)}
}

// Router maps URL paths to handlers.
type Router struct {
	routes map[string]HandlerFunc
}

// NewRouter returns an empty Router.
func NewRouter() *Router {
	return &Router{routes: make(map[string]HandlerFunc)}
}

// Add registers h for GET requests to path (e.g. "/dialogue").
func (r *Router) Add(path string, h HandlerFunc) {
	r.routes[path] = h
}

// serve is the shared core used by both the Lambda entry point and net/http.
func (r *Router) serve(ctx context.Context, method, path string, q url.Values) (status int, body []byte) {
	if method != http.MethodGet {
		return http.StatusMethodNotAllowed, mustJSON(map[string]string{"error": "method not allowed: this API answers GET requests only"})
	}
	h, ok := r.routes[path]
	if !ok {
		return http.StatusNotFound, mustJSON(map[string]string{"error": "unknown endpoint: " + path})
	}
	out, err := h(ctx, &Request{Query: q})
	if err != nil {
		var ce *ClientError
		if errors.As(err, &ce) {
			return http.StatusBadRequest, mustJSON(map[string]string{"error": ce.Msg})
		}
		// Log the real error server-side (CloudWatch in Lambda, stderr locally);
		// the client only gets a generic message.
		log.Printf("handler %s %s: %v", method, path, err)
		return http.StatusInternalServerError, mustJSON(map[string]string{"error": "internal error"})
	}
	return http.StatusOK, mustJSON(out)
}

// Handle is the AWS Lambda entry point (API Gateway REST API proxy event).
func (r *Router) Handle(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	q := make(url.Values, len(req.MultiValueQueryStringParameters))
	for k, vs := range req.MultiValueQueryStringParameters {
		q[k] = vs
	}
	for k, v := range req.QueryStringParameters {
		if _, ok := q[k]; !ok {
			q[k] = []string{v}
		}
	}
	status, body := r.serve(ctx, req.HTTPMethod, req.Path, q)
	return events.APIGatewayProxyResponse{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(body),
	}, nil
}

// ServeHTTP lets you run the same routes locally with net/http (see cmd/local).
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	status, body := r.serve(req.Context(), req.Method, req.URL.Path, req.URL.Query())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil { // should never happen for our plain structs/maps
		return []byte(`{"error":"encoding response"}`)
	}
	return b
}
