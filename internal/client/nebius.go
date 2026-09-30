package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type NebiusClient struct {
	APIKey   string
	BaseURL  string
	Model    string
	HTTP     *http.Client
	LastTTFT float64
	LastTPS  float64
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	Stream      bool          `json:"stream"`
}

type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type StreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

type PatchSuggestion struct {
	Analysis   string `json:"analysis"`
	TargetFile string `json:"target_file"`
	DiffPatch  string `json:"diff_patch"`
}

func NewNebiusClient() *NebiusClient {
	apiKey := os.Getenv("NEBIUS_API_KEY")
	baseURL := os.Getenv("NEBIUS_BASE_URL")
	model := os.Getenv("NEBIUS_MODEL")

	if baseURL == "" {
		if apiKey != "" {
			baseURL = "https://api.tokenfactory.nebius.com/v1"
		} else {
			baseURL = "http://192.168.2.115:8317/v1"
		}
	}

	if model == "" {
		if apiKey != "" {
			model = "nvidia/nemotron-3-ultra"
		} else {
			model = "gemini-3.8-flash-high"
		}
	}

	if apiKey == "" {
		// Try reading local api_key from ~/.hermes/config.yaml
		home, _ := os.UserHomeDir()
		configPath := filepath.Join(home, ".hermes", "config.yaml")
		if data, err := os.ReadFile(configPath); err == nil {
			re := regexp.MustCompile(`api_key:\s*([^\s\n]+)`)
			if m := re.FindStringSubmatch(string(data)); len(m) > 1 {
				apiKey = strings.TrimSpace(m[1])
			}
		}
	}

	return &NebiusClient{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
		HTTP:    &http.Client{Timeout: 120 * time.Second},
	}
}

// StreamCompletion sends a chat request and calls onToken for each incoming streaming token.
func (c *NebiusClient) StreamCompletion(ctx context.Context, messages []ChatMessage, onToken func(string)) (string, int, int, error) {
	reqBody := ChatRequest{
		Model:       c.Model,
		Messages:    messages,
		Temperature: 0.1,
		Stream:      true,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, 0, err
	}

	url := strings.TrimRight(c.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return "", 0, 0, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	startTime := time.Now()
	var firstTokenTime time.Time
	tokenCount := 0
	var fullText strings.Builder

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, 0, fmt.Errorf("nebius api error %d: %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		raw := strings.TrimPrefix(line, "data: ")
		if strings.TrimSpace(raw) == "[DONE]" {
			break
		}

		var chunk StreamChunk
		if err := json.Unmarshal([]byte(raw), &chunk); err == nil {
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				if firstTokenTime.IsZero() {
					firstTokenTime = time.Now()
					c.LastTTFT = time.Since(startTime).Seconds()
				}
				token := chunk.Choices[0].Delta.Content
				fullText.WriteString(token)
				tokenCount++
				if onToken != nil {
					onToken(token)
				}
			}
		}
	}

	totalDuration := time.Since(startTime).Seconds()
	if totalDuration > 0 && tokenCount > 0 {
		c.LastTPS = float64(tokenCount) / totalDuration
	}

	// Approximate token counts
	promptTokens := len(fmt.Sprintf("%v", messages)) / 4
	return fullText.String(), promptTokens, tokenCount, nil
}

// DiagnoseAndPatch asks Nemotron to analyze the failure and output a surgical unified diff.
func (c *NebiusClient) DiagnoseAndPatch(ctx context.Context, testCmd, stdout, stderr, codeContext, docsContext, archetypeContext string, onToken func(string)) (*PatchSuggestion, int, int, error) {
	prompt := fmt.Sprintf(`You are an autonomous senior Principal Engineer on Nebius Token Factory.
Fix the broken codebase by generating a surgical Unified Diff patch.

[TEST COMMAND]
%s

[FAILING OUTPUT]
%s

[DETERMINISTIC DEFECT CLASSIFICATION & CONSTRAINTS (ALIBABA OCR HYBRID)]
%s

[SOURCE CODE CONTEXT]
%s

[OFFICIAL DOCUMENTATION (TAVILY GROUNDING)]
%s

INSTRUCTIONS:
1. Provide a concise root cause analysis matching the defect archetype.
2. Output the exact relative target file path in [TARGET_FILE]path/to/file[/TARGET_FILE].
3. Output the exact, valid unified diff patch in a `+"```diff"+` block starting with:
--- a/path/to/file
+++ b/path/to/file
@@ ... @@
Do NOT omit the unified diff block.`, testCmd, stderr+"\n"+stdout, archetypeContext, codeContext, docsContext)

	messages := []ChatMessage{
		{Role: "system", Content: "You are an autonomous software repair agent. You generate precise unified diff patches that compile and pass tests."},
		{Role: "user", Content: prompt},
	}

	fullText, pTokens, cTokens, err := c.StreamCompletion(ctx, messages, onToken)
	if err != nil {
		return nil, 0, 0, err
	}

	targetFile := ""
	reTarget := regexp.MustCompile(`\[TARGET_FILE\](.*?)\[/TARGET_FILE\]`)
	if m := reTarget.FindStringSubmatch(fullText); len(m) > 1 {
		targetFile = strings.TrimSpace(m[1])
	}

	diffPatch := ""
	reDiff := regexp.MustCompile("(?s)```(?:diff|patch)?\r?\n(.*?)```")
	if m := reDiff.FindStringSubmatch(fullText); len(m) > 1 {
		diffPatch = strings.TrimSpace(m[1])
	} else if strings.Contains(fullText, "--- a/") && strings.Contains(fullText, "+++ b/") {
		startIdx := strings.Index(fullText, "--- a/")
		diffPatch = strings.TrimSpace(fullText[startIdx:])
	}

	if diffPatch == "" {
		fmt.Printf("[DEBUG RAW LLM RESPONSE]\n%s\n[/DEBUG RAW LLM RESPONSE]\n", fullText)
	}

	return &PatchSuggestion{
		Analysis:   fullText,
		TargetFile: targetFile,
		DiffPatch:  diffPatch,
	}, pTokens, cTokens, nil
}
