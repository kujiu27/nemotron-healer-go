package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/kujiu27/nemotron-healer-go/internal/sandbox"
)

type NebiusClient struct {
	APIKey           string
	BaseURL          string
	Model            string
	FastModel        string
	ReasoningModel   string
	HTTP             *http.Client
	LastTTFT         float64
	LastTPS          float64
	LastThoughtChain string
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type ChatRequest struct {
	Model         string         `json:"model"`
	Messages      []ChatMessage  `json:"messages"`
	Temperature   float64        `json:"temperature"`
	Stream        bool           `json:"stream"`
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`
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
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content,omitempty"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

type PatchSuggestion struct {
	Analysis     string   `json:"analysis"`
	ThoughtChain string   `json:"thought_chain,omitempty"`
	TargetFile   string   `json:"target_file"`
	TargetFiles  []string `json:"target_files,omitempty"`
	DiffPatch    string   `json:"diff_patch"`
}

// NewNebiusClient builds the inference client.
// Defaults comply with the Nebius x NVIDIA hackathon rules:
// Nebius Token Factory endpoint + NVIDIA Nemotron open models.
// Model ID verified against the official catalog (tokenfactory.nebius.com/model-catalog.md):
// nvidia/Nemotron-3-Ultra-550b-a55b — $1.00/1M input, $3.00/1M output.
// Local/self-hosted gateways are opt-in via NEBIUS_BASE_URL (+ NEBIUS_MODEL, optional NEBIUS_API_KEY).
func NewNebiusClient() *NebiusClient {
	apiKey := os.Getenv("NEBIUS_API_KEY")

	baseURL := os.Getenv("NEBIUS_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.tokenfactory.nebius.com/v1"
	}

	reasoningModel := "nvidia/Nemotron-3-Ultra-550b-a55b"
	if envModel := os.Getenv("NEBIUS_MODEL"); envModel != "" {
		reasoningModel = envModel
	}

	fastModel := "meta-llama/Meta-Llama-3.1-8B-Instruct"
	if envFast := os.Getenv("NEBIUS_FAST_MODEL"); envFast != "" {
		fastModel = envFast
	}

	return &NebiusClient{
		APIKey:         apiKey,
		BaseURL:        baseURL,
		Model:          reasoningModel,
		FastModel:      fastModel,
		ReasoningModel: reasoningModel,
		HTTP:           &http.Client{Timeout: 120 * time.Second},
	}
}

// executeWithRetry wraps HTTP requests with exponential backoff and jitter for transient errors (429, 5xx)
func (c *NebiusClient) executeWithRetry(ctx context.Context, createReq func() (*http.Request, error)) (*http.Response, error) {
	maxRetries := 3
	baseDelay := 500 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := createReq()
		if err != nil {
			return nil, err
		}

		resp, err := c.HTTP.Do(req)
		if err != nil {
			if attempt == maxRetries || ctx.Err() != nil {
				return nil, err
			}
		} else if resp.StatusCode == http.StatusOK {
			return resp, nil
		} else if resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusBadGateway ||
			resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode == http.StatusGatewayTimeout ||
			resp.StatusCode == http.StatusInternalServerError {

			if attempt == maxRetries || ctx.Err() != nil {
				return resp, nil
			}

			retrySec := 0
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				fmt.Sscanf(ra, "%d", &retrySec)
			}
			resp.Body.Close()

			var backoff time.Duration
			if retrySec > 0 {
				backoff = time.Duration(retrySec) * time.Second
			} else {
				multiplier := 1 << attempt
				jitter := time.Duration(rand.Intn(250)) * time.Millisecond
				backoff = time.Duration(multiplier)*baseDelay + jitter
			}

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			continue
		} else {
			return resp, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(baseDelay * time.Duration(1<<attempt)):
		}
	}
	return nil, fmt.Errorf("exceeded max retries")
}

// StreamCompletion sends a chat request and calls onToken for each incoming streaming token.
func (c *NebiusClient) StreamCompletion(ctx context.Context, messages []ChatMessage, onToken func(string)) (string, int, int, error) {
	return c.StreamCompletionWithTemp(ctx, messages, 0.1, onToken)
}

// StreamCompletionWithTemp sends a chat request with a configurable sampling temperature.
func (c *NebiusClient) StreamCompletionWithTemp(ctx context.Context, messages []ChatMessage, temperature float64, onToken func(string)) (string, int, int, error) {
	reqBody := ChatRequest{
		Model:         c.Model,
		Messages:      messages,
		Temperature:   temperature,
		Stream:        true,
		StreamOptions: &StreamOptions{IncludeUsage: true},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, 0, err
	}

	url := strings.TrimRight(c.BaseURL, "/") + "/chat/completions"
	newReq := func() (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/json")
		if c.APIKey != "" {
			r.Header.Set("Authorization", "Bearer "+c.APIKey)
		}
		return r, nil
	}

	startTime := time.Now()
	var firstTokenTime time.Time
	tokenCount := 0
	var fullText strings.Builder
	var thoughtText strings.Builder
	c.LastThoughtChain = ""
	serverPromptTokens := 0
	serverCompletionTokens := 0

	resp, err := c.executeWithRetry(ctx, newReq)
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
			if chunk.Usage != nil {
				serverPromptTokens = chunk.Usage.PromptTokens
				serverCompletionTokens = chunk.Usage.CompletionTokens
			}
			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				if delta.ReasoningContent != "" {
					thoughtText.WriteString(delta.ReasoningContent)
				}
				content := delta.Content
				if content == "" && delta.ReasoningContent != "" {
					content = delta.ReasoningContent
				}
				if content != "" {
					if firstTokenTime.IsZero() {
						firstTokenTime = time.Now()
						c.LastTTFT = time.Since(startTime).Seconds()
					}
					fullText.WriteString(content)
					tokenCount++
					if onToken != nil {
						onToken(content)
					}
				}
			}
		}
	}

	c.LastThoughtChain = strings.TrimSpace(thoughtText.String())

	totalDuration := time.Since(startTime).Seconds()
	if totalDuration > 0 && tokenCount > 0 {
		c.LastTPS = float64(tokenCount) / totalDuration
	}

	promptTokens := serverPromptTokens
	if promptTokens == 0 {
		promptTokens = len(fmt.Sprintf("%v", messages)) / 4
	}
	completionTokens := serverCompletionTokens
	if completionTokens == 0 {
		completionTokens = tokenCount
	}

	return fullText.String(), promptTokens, completionTokens, nil
}

// DiagnoseAndPatch asks Nemotron to analyze the failure and output a surgical unified diff.
func (c *NebiusClient) DiagnoseAndPatch(ctx context.Context, testCmd, stdout, stderr, codeContext, docsContext, archetypeContext string, failedHistory []string, onToken func(string)) (*PatchSuggestion, int, int, error) {
	var failureFeedback string
	if len(failedHistory) > 0 {
		var fb strings.Builder
		fb.WriteString("\n[PREVIOUS FAILED ATTEMPTS & NEGATIVE CONSTRAINTS (DO NOT REPEAT)]\n")
		fb.WriteString("The following patch attempts previously failed verification or introduced regressions. You MUST NOT repeat these mistakes:\n")
		for i, fh := range failedHistory {
			fb.WriteString(fmt.Sprintf("--- Failed Attempt #%d ---\n%s\n", i+1, fh))
		}
		fb.WriteString("INSTRUCTION: Analyze why the previous attempts failed. Formulate a fundamentally different, sound architectural repair.\n")
		failureFeedback = fb.String()
	}

	prompt := fmt.Sprintf(`You are an autonomous senior Principal Engineer on Nebius Token Factory.
Fix the broken codebase by generating a surgical Unified Diff patch.

[TEST COMMAND]
%s

[FAILING OUTPUT]
%s

[DETERMINISTIC DEFECT CLASSIFICATION & CONSTRAINTS (ARCHETYPE RULE ENGINE)]
%s
%s
[SOURCE CODE CONTEXT]
%s

[OFFICIAL DOCUMENTATION (TAVILY GROUNDING)]
%s

INSTRUCTIONS:
1. Provide a concise root cause analysis matching the defect archetype.
2. Output the exact relative target file path in [TARGET_FILE]path/to/file[/TARGET_FILE]. If modifying multiple files, output multiple [TARGET_FILE]path[/TARGET_FILE] tags or specify them directly in the multi-file unified diff headers.
3. Output the exact, valid atomic unified diff patch in a `+"```diff"+` block starting with:
--- a/path/to/file
+++ b/path/to/file
@@ ... @@
Do NOT omit the unified diff block.`, testCmd, stderr+"\n"+stdout, archetypeContext, failureFeedback, codeContext, docsContext)

	messages := []ChatMessage{
		{Role: "system", Content: "You are an autonomous software repair agent. You generate precise unified diff patches that compile and pass tests."},
		{Role: "user", Content: prompt},
	}

	// Adaptive Temperature Annealing:
	// Turn 1 (len(failedHistory) == 0): T = 0.25 (exploration across candidate hypotheses)
	// Turn 2+ (len(failedHistory) > 0): T = 0.05 (deterministic convergence on targeted surgical repair)
	temp := 0.25
	if len(failedHistory) > 0 {
		temp = 0.05
	}

	fullText, pTokens, cTokens, err := c.StreamCompletionWithTemp(ctx, messages, temp, onToken)
	if err != nil {
		return nil, 0, 0, err
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

	targetFiles := sandbox.ExtractModifiedFiles(diffPatch)
	reTarget := regexp.MustCompile(`\[TARGET_FILE\](.*?)\[/TARGET_FILE\]`)
	for _, m := range reTarget.FindAllStringSubmatch(fullText, -1) {
		if len(m) > 1 {
			f := strings.TrimSpace(m[1])
			if f != "" {
				found := false
				for _, tf := range targetFiles {
					if tf == f {
						found = true
						break
					}
				}
				if !found {
					targetFiles = append([]string{f}, targetFiles...)
				}
			}
		}
	}

	targetFile := ""
	if len(targetFiles) > 0 {
		targetFile = targetFiles[0]
	}

	thoughtChain := c.LastThoughtChain
	if thoughtChain == "" {
		reThought := regexp.MustCompile(`(?s)<thought>(.*?)</thought>`)
		if m := reThought.FindStringSubmatch(fullText); len(m) > 1 {
			thoughtChain = strings.TrimSpace(m[1])
		}
	}

	return &PatchSuggestion{
		Analysis:     fullText,
		ThoughtChain: thoughtChain,
		TargetFile:   targetFile,
		TargetFiles:  targetFiles,
		DiffPatch:    diffPatch,
	}, pTokens, cTokens, nil
}

type FastTriageResult struct {
	DecisiveErrorLine     string `json:"decisive_error_line"`
	HypothesizedRootCause string `json:"hypothesized_root_cause"`
	RecommendedQuery      string `json:"recommended_query"`
}

// FastTriage calls the fast lightweight tier model (<150ms) to extract quick diagnostic insights and search query.
func (c *NebiusClient) FastTriage(ctx context.Context, errorTrace string) (*FastTriageResult, int, int, error) {
	if c.FastModel == "" {
		return nil, 0, 0, nil
	}

	prompt := fmt.Sprintf(`You are a fast pre-flight code triage agent.
Analyze the following test failure traceback. Extract:
1. The decisive root error line.
2. A 1-sentence hypothesis of the root cause.
3. A 4-word search query for official documentation.

Respond in exact format:
ERROR_LINE: <line>
CAUSE: <sentence>
QUERY: <query>

[TRACE]
%s`, errorTrace)

	reqBody := ChatRequest{
		Model:       c.FastModel,
		Messages:    []ChatMessage{{Role: "user", Content: prompt}},
		Temperature: 0.0,
		Stream:      false,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, 0, err
	}

	url := strings.TrimRight(c.BaseURL, "/") + "/chat/completions"
	newReq := func() (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/json")
		if c.APIKey != "" {
			r.Header.Set("Authorization", "Bearer "+c.APIKey)
		}
		return r, nil
	}

	resp, err := c.executeWithRetry(ctx, newReq)
	if err != nil {
		return nil, 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, 0, fmt.Errorf("fast triage status: %d", resp.StatusCode)
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, 0, 0, err
	}

	if len(chatResp.Choices) == 0 {
		return nil, chatResp.Usage.PromptTokens, chatResp.Usage.CompletionTokens, nil
	}

	text := chatResp.Choices[0].Message.Content
	res := &FastTriageResult{}

	lines := strings.Split(text, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "ERROR_LINE:") {
			res.DecisiveErrorLine = strings.TrimSpace(strings.TrimPrefix(l, "ERROR_LINE:"))
		} else if strings.HasPrefix(l, "CAUSE:") {
			res.HypothesizedRootCause = strings.TrimSpace(strings.TrimPrefix(l, "CAUSE:"))
		} else if strings.HasPrefix(l, "QUERY:") {
			res.RecommendedQuery = strings.TrimSpace(strings.TrimPrefix(l, "QUERY:"))
		}
	}

	return res, chatResp.Usage.PromptTokens, chatResp.Usage.CompletionTokens, nil
}
