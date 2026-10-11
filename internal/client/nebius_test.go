package client

import (
	"context"
	"encoding/json"
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

func TestNebiusClient_TelemetryConcurrency(t *testing.T) {
	client := &NebiusClient{
		LastTTFT:         0.12,
		LastTPS:          45.6,
		LastThoughtChain: "test-thought",
	}

	done := make(chan bool)
	// Reader goroutine
	go func() {
		for range 100 {
			ttft, tps, tc := client.Telemetry()
			if ttft < 0 || tps < 0 || tc == "" {
				t.Errorf("invalid telemetry read")
			}
		}
		done <- true
	}()

	// Writer goroutine
	go func() {
		for range 100 {
			client.mu.Lock()
			client.LastTTFT = 0.15
			client.LastTPS = 50.0
			client.LastThoughtChain = "updated"
			client.mu.Unlock()
		}
		done <- true
	}()

	<-done
	<-done
}

func TestDiagnoseAndPatch_IncludesBlastRadiusConstraints(t *testing.T) {
	var capturedPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, m := range req.Messages {
			if m.Role == "user" {
				capturedPrompt = m.Content
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"```diff\\n--- a/file.go\\n+++ b/file.go\\n@@ -1 +1 @@\\n-old\\n+new\\n```\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()

	client := &NebiusClient{
		BaseURL: server.URL,
		Model:   "nvidia/Nemotron-3-Ultra-550b-a55b",
		HTTP:    server.Client(),
	}

	blastCtx := "- Target Symbol: `file.go::Foo`\n- Blast Risk Level: HIGH (Risk Score: 0.85 / 1.00)\n- Direct Callers: main.go::Run"
	sug, _, _, err := client.DiagnoseAndPatch(context.Background(), "go test .", "out", "err", "ctx", "docs", "arch", blastCtx, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sug == nil || sug.DiffPatch == "" {
		t.Fatalf("expected patch suggestion")
	}
	if !strings.Contains(capturedPrompt, "[AST SYMBOL BLAST RADIUS & CALLER CONSTRAINTS]") {
		t.Errorf("expected prompt to contain blast radius header, got:\n%s", capturedPrompt)
	}
	if !strings.Contains(capturedPrompt, "file.go::Foo") {
		t.Errorf("expected prompt to contain target symbol, got:\n%s", capturedPrompt)
	}
	if !strings.Contains(capturedPrompt, "main.go::Run") {
		t.Errorf("expected prompt to contain caller, got:\n%s", capturedPrompt)
	}
}
