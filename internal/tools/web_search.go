package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
)

const tavilySearchURL = "https://api.tavily.com/search"

// WebSearchTool implements Tool for searching the web using Tavily.
type WebSearchTool struct {
	client  *http.Client
	apiKey  string
	baseURL string
}

func NewWebSearchTool(apiKey string) WebSearchTool {
	return WebSearchTool{
		client:  &http.Client{Timeout: 20 * time.Second},
		apiKey:  apiKey,
		baseURL: tavilySearchURL,
	}
}

func (WebSearchTool) Name() string {
	return "web_search"
}

func (w WebSearchTool) Definition() openai.ChatCompletionToolUnionParam {
	return newFunctionTool(w.Name(), "Search the web for current information. Returns results with titles, URLs, content snippets, and an optional AI-generated answer.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]string{
				"type":        "string",
				"description": "The search query, e.g., 'Go 1.23 release notes' or 'how to use context.WithTimeout'",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return (default: 5, max: 10)",
				"default":     5,
			},
			"topic": map[string]any{
				"type":        "string",
				"description": "Search category: 'general' for broad searches, 'news' for recent events/updates (default: general)",
				"enum":        []string{"general", "news"},
				"default":     "general",
			},
			"include_answer": map[string]any{
				"type":        "boolean",
				"description": "Include a short AI-generated answer summarising the results (default: true)",
				"default":     true,
			},
		},
		"required": []string{"query"},
	})
}

// tavilyRequest is the JSON body sent to Tavily /search.
type tavilyRequest struct {
	Query         string `json:"query"`
	MaxResults    int    `json:"max_results"`
	Topic         string `json:"topic"`
	SearchDepth   string `json:"search_depth"`
	IncludeAnswer bool   `json:"include_answer"`
}

// tavilyResponse maps the fields we care about from Tavily's response.
type tavilyResponse struct {
	Query   string         `json:"query"`
	Answer  string         `json:"answer"`
	Results []tavilyResult `json:"results"`
}

type tavilyResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

type SearchResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

type SearchResponse struct {
	Answer  string         `json:"answer,omitempty"`
	Results []SearchResult `json:"results"`
}

func (w WebSearchTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("missing or invalid 'query' argument")
	}

	fmt.Printf("web_search: searching for %q\n", query)

	maxResults := 5
	if v, ok := args["max_results"]; ok {
		switch n := v.(type) {
		case float64:
			maxResults = int(n)
		case int:
			maxResults = n
		}
	}
	if maxResults < 1 {
		maxResults = 1
	}
	if maxResults > 10 {
		maxResults = 10
	}

	topic := "general"
	if v, ok := args["topic"].(string); ok && v != "" {
		topic = v
	}

	includeAnswer := true
	if v, ok := args["include_answer"].(bool); ok {
		includeAnswer = v
	}

	reqBody := tavilyRequest{
		Query:         query,
		MaxResults:    maxResults,
		Topic:         topic,
		SearchDepth:   "basic",
		IncludeAnswer: includeAnswer,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.baseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+w.apiKey)

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tavily API error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var tavily tavilyResponse
	if err := json.Unmarshal(respBytes, &tavily); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	out := SearchResponse{
		Answer:  tavily.Answer,
		Results: make([]SearchResult, 0, len(tavily.Results)),
	}
	for _, r := range tavily.Results {
		out.Results = append(out.Results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
			Score:   r.Score,
		})
	}

	if len(out.Results) == 0 {
		return fmt.Sprintf("No results found for query: %q", query), nil
	}

	return out, nil
}
