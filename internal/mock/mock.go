package mock

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
)

// Server provides an in-process, labeled mock of Nebius Token Factory and Tavily APIs.
type Server struct {
	*httptest.Server
}

// StartServer launches a pure Go mock server on a random localhost port.
// Every response carries the X-Nemotron-Healer: MOCK header.
func StartServer() *Server {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Nemotron-Healer", "MOCK")
		path := r.URL.Path

		if r.Method == http.MethodGet && (path == "/" || path == "/v1" || path == "/models" || path == "/v1/models") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[{"id":"nvidia/Nemotron-3-Ultra-550b-a55b"},{"id":"nvidia/Nemotron-3-Nano-30B-A3B"}]}`))
			return
		}

		if r.Method == http.MethodPost && (path == "/search" || path == "/v1/search") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results":[{"title":"tidwall/gjson Documentation","url":"https://github.com/tidwall/gjson","content":"GJSON path syntax supports empty string queries and comparisons.","score":0.95}]}`))
			return
		}

		if r.Method == http.MethodPost && (path == "/extract" || path == "/v1/extract") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results":[{"url":"https://github.com/tidwall/gjson","raw_content":"GJSON path syntax supports empty string queries and comparisons."}]}`))
			return
		}

		if r.Method == http.MethodPost && (path == "/chat/completions" || path == "/v1/chat/completions") {
			var chatReq struct {
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
				Stream bool `json:"stream"`
			}
			_ = json.NewDecoder(r.Body).Decode(&chatReq)

			allContent := ""
			for _, m := range chatReq.Messages {
				allContent += " " + m.Content
			}
			lower := strings.ToLower(allContent)
			// Verified surgical patch for samples/external_gjson
			reply := "```diff\n--- a/gjson.go\n+++ b/gjson.go\n@@ -761,7 +761,7 @@\n-\t\t\t\t\tif len(value) > 2 && value[0] == '\"' &&\n+\t\t\t\t\tif len(value) >= 2 && value[0] == '\"' &&\n \t\t\t\t\t\tvalue[len(value)-1] == '\"' {\n```\n[TARGET_FILE]gjson.go[/TARGET_FILE]"

			// Grounding detection: in baseline ablation (DisableGrounding = true), docsContext is omitted.
			// Without Tavily documentation, the baseline model emits an overfitted / incorrect patch.
			isGrounded := strings.Contains(allContent, "GJSON path syntax supports empty string")

			if strings.Contains(lower, "triage") || strings.Contains(allContent, "ERROR_LINE") || strings.Contains(allContent, "4-word") {
				reply = "ERROR_LINE: len(value) > 2\nCAUSE: array-path parser rejects the empty quoted string\nQUERY: gjson empty string query operator"
			} else if strings.Contains(lower, "adversarial") || strings.Contains(allContent, "Red-Teamer") || strings.Contains(lower, "stress") {
				reply = "```go\npackage gjson\n\nimport \"testing\"\n\nfunc TestAdversarialEmptyQueryMock(t *testing.T) {\n\tif Get(`[\"a\",\"\"]`, `#(!=\"\")#`).Raw != `[\"a\"]` {\n\t\tt.Fatal(\"empty-string filter mismatch\")\n\t}\n}\n```"
			} else if !isGrounded {
				// Naive ungrounded baseline patch: attempts an incorrect condition that fails TestEmptyValueQuery
				reply = "```diff\n--- a/gjson.go\n+++ b/gjson.go\n@@ -761,7 +761,7 @@\n-\t\t\t\t\tif len(value) > 2 && value[0] == '\"' &&\n+\t\t\t\t\tif len(value) > 5 && value[0] == '\"' &&\n \t\t\t\t\t\tvalue[len(value)-1] == '\"' {\n```\n[TARGET_FILE]gjson.go[/TARGET_FILE]"
			}
			if chatReq.Stream {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				w.WriteHeader(http.StatusOK)
				flusher, _ := w.(http.Flusher)

				chunk := map[string]interface{}{
					"choices": []map[string]interface{}{
						{"delta": map[string]string{"content": reply}},
					},
					"usage": map[string]int{
						"prompt_tokens":     120,
						"completion_tokens": 45,
					},
				}
				d, _ := json.Marshal(chunk)
				fmt.Fprintf(w, "data: %s\n\n", d)
				fmt.Fprintf(w, "data: [DONE]\n\n")
				if flusher != nil {
					flusher.Flush()
				}
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			respObj := map[string]interface{}{
				"choices": []map[string]interface{}{
					{"message": map[string]string{"content": reply}},
				},
				"usage": map[string]int{
					"prompt_tokens":     120,
					"completion_tokens": 45,
				},
			}
			_ = json.NewEncoder(w).Encode(respObj)
			return
		}

		http.NotFound(w, r)
	}

	ts := httptest.NewServer(http.HandlerFunc(handler))
	return &Server{Server: ts}
}
