package mock

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMockServer_Endpoints(t *testing.T) {
	server := StartServer()
	defer server.Close()

	// 1. GET /models
	resp, err := http.Get(server.URL + "/v1/models")
	if err != nil {
		t.Fatalf("GET /models failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 from /models, got %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Nemotron-Healer") != "MOCK" {
		t.Errorf("missing X-Nemotron-Healer: MOCK header")
	}

	// 2. POST /chat/completions (stream: false, triage)
	triageBody := []byte(`{"messages":[{"role":"user","content":"pre-flight triage error"}],"stream":false}`)
	resp, err = http.Post(server.URL+"/v1/chat/completions", "application/json", bytes.NewReader(triageBody))
	if err != nil {
		t.Fatalf("POST /chat/completions failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "ERROR_LINE") || !strings.Contains(string(body), "QUERY") {
		t.Errorf("expected triage format in response, got %s", string(body))
	}

	// 3. POST /chat/completions (stream: true, diff patch)
	diffBody := []byte(`{"messages":[{"role":"user","content":"generate surgical unified diff patch"}],"stream":true}`)
	resp, err = http.Post(server.URL+"/v1/chat/completions", "application/json", bytes.NewReader(diffBody))
	if err != nil {
		t.Fatalf("POST /chat/completions streaming failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "data: ") || !strings.Contains(string(body), "--- a/gjson.go") {
		t.Errorf("expected SSE streaming diff, got %s", string(body))
	}

	// 4. POST /search
	searchBody := []byte(`{"query":"gjson empty string"}`)
	resp, err = http.Post(server.URL+"/search", "application/json", bytes.NewReader(searchBody))
	if err != nil {
		t.Fatalf("POST /search failed: %v", err)
	}
	defer resp.Body.Close()
	var searchResp struct {
		Results []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil || len(searchResp.Results) == 0 {
		t.Errorf("expected search results, err: %v", err)
	}

	// 5. POST /extract
	extractBody := []byte(`{"urls":["https://github.com/tidwall/gjson"]}`)
	resp, err = http.Post(server.URL+"/extract", "application/json", bytes.NewReader(extractBody))
	if err != nil {
		t.Fatalf("POST /extract failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "raw_content") {
		t.Errorf("expected raw_content in extract response, got %s", string(body))
	}
}

func TestMockServer_CascadeDispatch(t *testing.T) {
	server := StartServer()
	defer server.Close()

	cascadeBody := []byte(`{"messages":[{"role":"user","content":"fix deadlock in service.py BankingService"}],"stream":false}`)
	resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", bytes.NewReader(cascadeBody))
	if err != nil {
		t.Fatalf("POST /chat/completions for cascade failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "--- a/service.py") || !strings.Contains(string(body), "async with self.engine.lock:") {
		t.Errorf("expected service.py patch for cascade, got %s", string(body))
	}
}
