package engine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsAuthError(t *testing.T) {
	cases := []struct {
		err      error
		expected bool
	}{
		{errors.New("nebius api error 401: unauthorized"), true},
		{errors.New("HTTP 403 Forbidden"), true},
		{errors.New("token is not present"), true},
		{errors.New("Couldn't authenticate"), true},
		{errors.New("connection timeout"), false},
		{errors.New("syntax error in generated patch"), false},
		{nil, false},
	}

	for _, c := range cases {
		got := isAuthError(c.err)
		if got != c.expected {
			t.Errorf("isAuthError(%v) = %v; want %v", c.err, got, c.expected)
		}
	}
}

func TestAgentRun_AuthFastFail(t *testing.T) {
	tmpDir := t.TempDir()

	// Create broken file that fails reproduction
	mainGo := filepath.Join(tmpDir, "main.go")
	_ = os.WriteFile(mainGo, []byte("package main\nfunc main() { panic(\"boom\") }\n"), 0644)

	// Mock server that returns 401 Unauthorized
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"Couldn't authenticate. Reason: token is not present"}`))
	}))
	defer server.Close()

	agent := NewAgent(tmpDir, "go run main.go", 5, nil, nil)
	agent.Nebius.BaseURL = server.URL
	agent.Nebius.APIKey = ""

	start := time.Now()
	session, err := agent.Run(context.Background())
	duration := time.Since(start)

	// Must fail fast on Turn 1 without burning all 5 turns
	if err == nil {
		t.Fatalf("expected auth error, got nil")
	}
	if session.CurrentTurn > 1 {
		t.Errorf("expected fast-fail on Turn 1, but ran %d turns", session.CurrentTurn)
	}
	if duration > 5*time.Second {
		t.Errorf("expected fast fail in <5s, took %v", duration)
	}
	if session.IsResolved {
		t.Errorf("session must not be marked resolved on auth failure")
	}
}
