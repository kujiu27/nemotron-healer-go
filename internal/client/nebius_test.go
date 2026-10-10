package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNebiusRetryBackoffOn429(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		curr := atomic.AddInt32(&attempts, 1)
		if curr < 3 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error": "rate limit exceeded"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices": [{"message": {"content": "SUCCESS"}}]}`))
	}))
	defer server.Close()

	client := &NebiusClient{
		BaseURL: server.URL,
		Model:   "nvidia/Nemotron-3-Ultra-550b-a55b",
		HTTP:    server.Client(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	newReq := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, "POST", server.URL, nil)
	}

	resp, err := client.executeWithRetry(ctx, newReq)
	if err != nil {
		t.Fatalf("expected retry to eventually succeed, got err: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got: %d", resp.StatusCode)
	}

	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("expected exactly 3 attempts (2 retries), got: %d", atomic.LoadInt32(&attempts))
	}
}

func TestStreamCompletion_LargePayloadSurvivesDefaultScannerLimit(t *testing.T) {
	// Generate a single SSE data line that exceeds the default 64KB bufio.MaxScanTokenSize (e.g. 128KB payload)
	largeContent := strings.Repeat("A", 128*1024)
	sseChunk := fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\ndata: [DONE]\n\n", largeContent)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sseChunk))
	}))
	defer server.Close()

	client := &NebiusClient{
		BaseURL: server.URL,
		Model:   "nvidia/Nemotron-3-Ultra-550b-a55b",
		HTTP:    server.Client(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	received := ""
	fullText, _, _, err := client.StreamCompletion(ctx, []ChatMessage{{Role: "user", Content: "test"}}, func(tok string) {
		received += tok
	})
	if err != nil {
		t.Fatalf("StreamCompletion failed on >64KB SSE chunk: %v", err)
	}

	if len(fullText) != len(largeContent) {
		t.Fatalf("expected fullText length %d, got %d", len(largeContent), len(fullText))
	}
	if len(received) != len(largeContent) {
		t.Fatalf("expected received callback tokens length %d, got %d", len(largeContent), len(received))
	}
}
