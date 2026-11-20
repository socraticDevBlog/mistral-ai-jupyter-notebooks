package lambdaapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func testRouter() *Router {
	rt := NewRouter()
	rt.Add("/echo", func(ctx context.Context, r *Request) (any, error) {
		return map[string]string{"you_said": r.Query.Get("q")}, nil
	})
	rt.Add("/bad", func(ctx context.Context, r *Request) (any, error) {
		return nil, BadRequest("missing q")
	})
	rt.Add("/boom", func(ctx context.Context, r *Request) (any, error) {
		return nil, errors.New("kaput")
	})
	return rt
}

func callLambda(rt *Router, method, path string, query map[string]string) events.APIGatewayProxyResponse {
	resp, err := rt.Handle(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:            method,
		Path:                  path,
		QueryStringParameters: query,
	})
	if err != nil {
		panic(err)
	}
	return resp
}

func bodyOf(t *testing.T, resp events.APIGatewayProxyResponse) map[string]string {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal([]byte(resp.Body), &m); err != nil {
		t.Fatalf("body is not a JSON object: %q", resp.Body)
	}
	return m
}

func TestGetRouteReturnsJSON(t *testing.T) {
	resp := callLambda(testRouter(), http.MethodGet, "/echo", map[string]string{"q": "hi"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, resp.Body)
	}
	if got := bodyOf(t, resp)["you_said"]; got != "hi" {
		t.Errorf("you_said = %q, want %q", got, "hi")
	}
	if ct := resp.Headers["Content-Type"]; ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestNonGetRejected(t *testing.T) {
	resp := callLambda(testRouter(), http.MethodPost, "/echo", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", resp.StatusCode)
	}
}

func TestUnknownPath(t *testing.T) {
	resp := callLambda(testRouter(), http.MethodGet, "/nope", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestClientErrorIs400(t *testing.T) {
	resp := callLambda(testRouter(), http.MethodGet, "/bad", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if got := bodyOf(t, resp)["error"]; got != "missing q" {
		t.Errorf("error = %q", got)
	}
}

func TestInternalErrorIs500(t *testing.T) {
	resp := callLambda(testRouter(), http.MethodGet, "/boom", nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

func TestServeHTTPForLocalDev(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/echo?q=local", nil)
	rec := httptest.NewRecorder()
	testRouter().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != `{"you_said":"local"}` {
		t.Errorf("body = %s", rec.Body.String())
	}
}
