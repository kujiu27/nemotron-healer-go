package client

import (
	"bytes"
	"context"
	"encoding/json"
	"math/rand/v2"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type TavilyClient struct {
	APIKey  string
	BaseURL string
	HTTP    *http.Client
	Cache   map[string]*TavilySearchResponse
	Mu      sync.RWMutex
}

type TavilySearchRequest struct {
	APIKey            string   `json:"api_key"`
	Query             string   `json:"query"`
	SearchDepth       string   `json:"search_depth"`
	IncludeAnswer     bool     `json:"include_answer"`
	MaxResults        int      `json:"max_results"`
	IncludeRawContent bool     `json:"include_raw_content"`
	IncludeDomains    []string `json:"include_domains,omitempty"`
}

type TavilySearchResultItem struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

type TavilySearchResponse struct {
	Query   string                   `json:"query"`
	Answer  string                   `json:"answer"`
	Results []TavilySearchResultItem `json:"results"`
}

func NewTavilyClient() *TavilyClient {
	baseURL := os.Getenv("TAVILY_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.tavily.com"
	}
	return &TavilyClient{
		APIKey:  os.Getenv("TAVILY_API_KEY"),
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		Cache:   make(map[string]*TavilySearchResponse),
	}
}

func (t *TavilyClient) Search(ctx context.Context, query string, maxResults int) (*TavilySearchResponse, error) {
	t.Mu.RLock()
	if cached, ok := t.Cache[query]; ok {
		t.Mu.RUnlock()
		return cached, nil
	}
	t.Mu.RUnlock()
	if t.APIKey == "" {
		// No fabrication: without an API key, grounding is skipped entirely.
		fmt.Fprintln(os.Stderr, "[nemotron-healer] TAVILY_API_KEY not set — knowledge grounding disabled (no simulated results).")
		return nil, nil
	}

	reqBody := TavilySearchRequest{
		APIKey:            t.APIKey,
		Query:             query,
		SearchDepth:       "advanced",
		IncludeAnswer:     true,
		MaxResults:        maxResults,
		IncludeRawContent: false,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	newReq := func() (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, "POST", t.BaseURL+"/search", bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/json")
		return r, nil
	}

	resp, err := t.executeWithRetry(ctx, newReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tavily api returned %d", resp.StatusCode)
	}

	var searchResp TavilySearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	t.Mu.Lock()
	t.Cache[query] = &searchResp
	t.Mu.Unlock()

	return &searchResp, nil
}

type tavilyExtractRequest struct {
	APIKey string   `json:"api_key"`
	URLs   []string `json:"urls"`
}

type tavilyExtractResponse struct {
	Results []struct {
		URL        string `json:"url"`
		RawContent string `json:"raw_content"`
	} `json:"results"`
	FailedResults []struct {
		URL   string `json:"url"`
		Error string `json:"error"`
	} `json:"failed_results"`
}

// Extract fetches the raw full text of a page via the Tavily Extract API.
// Returns "" (not an error) when the URL is reported in failed_results so
// callers degrade to search snippets without noise.
func (t *TavilyClient) Extract(ctx context.Context, url string) (string, error) {
	if t.APIKey == "" || t.BaseURL == "" {
		return "", nil
	}
	data, err := json.Marshal(tavilyExtractRequest{APIKey: t.APIKey, URLs: []string{url}})
	if err != nil {
		return "", err
	}
	newReq := func() (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, "POST", t.BaseURL+"/extract", bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/json")
		return r, nil
	}

	resp, err := t.executeWithRetry(ctx, newReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tavily extract api returned %d", resp.StatusCode)
	}
	var exResp tavilyExtractResponse
	if err := json.NewDecoder(resp.Body).Decode(&exResp); err != nil {
		return "", err
	}
	for _, r := range exResp.Results {
		if r.URL == url && r.RawContent != "" {
			return r.RawContent, nil
		}
	}
	return "", nil
}
// Probe verifies API reachability and authentication with a minimal search query.
func (t *TavilyClient) Probe(ctx context.Context) (int, int64, error) {
	if t.APIKey == "" {
		return 0, 0, fmt.Errorf("TAVILY_API_KEY is not set")
	}
	start := time.Now()
	body := TavilySearchRequest{
		APIKey:      t.APIKey,
		Query:       "ping",
		MaxResults:  1,
		SearchDepth: "basic",
	}
	b, err := json.Marshal(body)
	if err != nil {
		return 0, 0, err
	}
	url := strings.TrimRight(t.BaseURL, "/") + "/search"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(b))
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	rtt := time.Since(start).Milliseconds()
	if err != nil {
		return 0, rtt, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, rtt, nil
}

func (t *TavilyClient) FormatContext(resp *TavilySearchResponse) string {
	if resp == nil || len(resp.Results) == 0 {
		return "No external documentation retrieved."
	}

	var sb strings.Builder
	if resp.Answer != "" {
		sb.WriteString(fmt.Sprintf("SUMMARY: %s\n\n", resp.Answer))
	}

	for i, r := range resp.Results {
		sb.WriteString(fmt.Sprintf("[%d] %s (%s)\n%s\n\n", i+1, r.Title, r.URL, r.Content))
	}
	return sb.String()
}

// executeWithRetry wraps HTTP requests with exponential backoff and jitter for transient errors (429, 5xx)
func (t *TavilyClient) executeWithRetry(ctx context.Context, createReq func() (*http.Request, error)) (*http.Response, error) {
	maxRetries := 3
	baseDelay := 100 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := createReq()
		if err != nil {
			return nil, err
		}

		resp, err := t.HTTP.Do(req)
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
				jitter := time.Duration(rand.IntN(100)) * time.Millisecond
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
