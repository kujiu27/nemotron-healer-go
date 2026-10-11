package mock

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func TestMockServer_AllBenchmarkCasesDispatch(t *testing.T) {
	server := StartServer()
	defer server.Close()

	cases := []struct {
		name      string
		prompt    string
		wantFile  string
		wantPatch string
	}{
		{
			name:      "AHB-01 account.py",
			prompt:    "fix concurrency in BankLedger account.py",
			wantFile:  "account.py",
			wantPatch: "self._lock = threading.Lock()",
		},
		{
			name:      "AHB-03 models.py",
			prompt:    "migrate pydantic v1 to v2 in models.py",
			wantFile:  "models.py",
			wantPatch: "field_validator",
		},
		{
			name:      "AHB-04 repo.py",
			prompt:    "prevent SQL injection in UserRepository repo.py",
			wantFile:  "repo.py",
			wantPatch: "WHERE username = ?",
		},
		{
			name:      "AHB-05 cache.go",
			prompt:    "fix data race in MetricCache cache.go",
			wantFile:  "cache.go",
			wantPatch: "sync.RWMutex",
		},
		{
			name:      "AHB-06 diff.go",
			prompt:    "fix diffmatchpatch lineHash in diffLinesToStrings",
			wantFile:  "diffmatchpatch/diff.go",
			wantPatch: "lineHash := map[string]int{}",
		},
		{
			name:      "AHB-07 unmarshaler.go",
			prompt:    "prevent infinite recursion in addFields unmarshaler.go TestUnmarshalRecursiveEmbedded",
			wantFile:  "unmarshaler.go",
			wantPatch: "visited := map[reflect.Type]bool{}",
		},
		{
			name:      "AHB-09 map_claims.go",
			prompt:    "validate aud claim in parseClaimsString map_claims.go",
			wantFile:  "map_claims.go",
			wantPatch: "ErrInvalidType",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"messages":[{"role":"user","content":%q}],"stream":false}`, tc.prompt))
			resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("POST failed: %v", err)
			}
			defer resp.Body.Close()
			respBytes, _ := io.ReadAll(resp.Body)
			s := string(respBytes)
			if !strings.Contains(s, tc.wantFile) || !strings.Contains(s, tc.wantPatch) {
				t.Errorf("expected %s patch with %s, got:\n%s", tc.wantFile, tc.wantPatch, s)
			}
		})
	}
}
