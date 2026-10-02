package client

import (
	"bytes"
	"context"
	"encoding/json"
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

	req, err := http.NewRequestWithContext(ctx, "POST", t.BaseURL+"/search", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.HTTP.Do(req)
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
	req, err := http.NewRequestWithContext(ctx, "POST", t.BaseURL+"/extract", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.HTTP.Do(req)
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
