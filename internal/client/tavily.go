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
	APIKey string
	HTTP   *http.Client
	Cache  map[string]*TavilySearchResponse
	Mu     sync.RWMutex
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
	return &TavilyClient{
		APIKey: os.Getenv("TAVILY_API_KEY"),
		HTTP:   &http.Client{Timeout: 30 * time.Second},
		Cache:  make(map[string]*TavilySearchResponse),
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
		// Simulation / Local Fallback when API key is missing
		return &TavilySearchResponse{
			Query:  query,
			Answer: "Simulated Tavily knowledge retrieval for: " + query,
			Results: []TavilySearchResultItem{
				{
					Title:   "Official Migration Guide & Reference",
					URL:     "https://docs.official.org/migration",
					Content: "Breaking change: in modern versions, replace deprecated configurations with modern idioms. Ensure correct parameters.",
					Score:   0.95,
				},
			},
		}, nil
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

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.tavily.com/search", bytes.NewReader(data))
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
