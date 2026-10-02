package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestTavily(handler http.HandlerFunc) *TavilyClient {
	srv := httptest.NewServer(handler)
	t := &TavilyClient{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		HTTP:    srv.Client(),
		Cache:   make(map[string]*TavilySearchResponse),
	}
	return t
}

func TestTavilyExtractSuccess(t *testing.T) {
	c := newTestTavily(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/extract" {
			t.Fatalf("path = %s, want /extract", r.URL.Path)
		}
		w.Write([]byte(`{"results":[{"url":"https://docs.example.com/pydantic","raw_content":"field_validator replaces validator"}]}`))
	})
	got, err := c.Extract(context.Background(), "https://docs.example.com/pydantic")
	if err != nil || got != "field_validator replaces validator" {
		t.Fatalf("Extract = %q, %v", got, err)
	}
}

func TestTavilyExtractFailedResultDegradesSilently(t *testing.T) {
	c := newTestTavily(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"results":[],"failed_results":[{"url":"https://x","error":"404"}]}`))
	})
	got, err := c.Extract(context.Background(), "https://x")
	if err != nil || got != "" {
		t.Fatalf("Extract = %q, %v; want empty, nil (honest degrade to snippets)", got, err)
	}
}

func TestTavilyExtractHTTPError(t *testing.T) {
	c := newTestTavily(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	got, err := c.Extract(context.Background(), "https://x")
	if err == nil || got != "" {
		t.Fatalf("Extract = %q, %v; want error on 429", got, err)
	}
}
