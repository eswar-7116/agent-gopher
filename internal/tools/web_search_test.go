package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebSearchTool_Execute(t *testing.T) {
	t.Run("successful search with all fields", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST request, got %q", r.Method)
			}

			var req tavilyRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("failed to decode request: %v", err)
			}

			if req.Query != "golang" {
				t.Errorf("expected query 'golang', got %q", req.Query)
			}

			resp := tavilyResponse{
				Answer: "Go is a programming language.",
				Results: []tavilyResult{
					{Title: "Go Programming Language", URL: "https://go.dev", Content: "The Go home page.", Score: 0.99},
					{Title: "Wikipedia: Go", URL: "https://en.wikipedia.org/wiki/Go_(programming_language)", Content: "Go is statically typed.", Score: 0.85},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		tool := WebSearchTool{
			client:  server.Client(),
			apiKey:  "test-key",
			baseURL: server.URL,
		}

		result, err := tool.Execute(context.Background(), map[string]any{"query": "golang", "max_results": 5})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		response, ok := result.(SearchResponse)
		if !ok {
			t.Fatalf("expected SearchResponse, got %T", result)
		}

		if response.Answer != "Go is a programming language." {
			t.Errorf("expected answer 'Go is a programming language.', got %q", response.Answer)
		}

		if len(response.Results) != 2 {
			t.Errorf("expected 2 results, got %d", len(response.Results))
		}

		if response.Results[0].Title != "Go Programming Language" {
			t.Errorf("expected first result title 'Go Programming Language', got %q", response.Results[0].Title)
		}
	})

	t.Run("respect max_results limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req tavilyRequest
			json.NewDecoder(r.Body).Decode(&req)
			
			results := []tavilyResult{
				{Title: "Result 1", Content: "Content 1"},
				{Title: "Result 2", Content: "Content 2"},
				{Title: "Result 3", Content: "Content 3"},
			}
			
			// Tavily would normally limit this, but we'll mock the response
			limit := req.MaxResults
			if limit > len(results) {
				limit = len(results)
			}
			
			resp := tavilyResponse{
				Results: results[:limit],
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		tool := WebSearchTool{
			client:  server.Client(),
			apiKey:  "test-key",
			baseURL: server.URL,
		}

		result, err := tool.Execute(context.Background(), map[string]any{"query": "test", "max_results": 2})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		response, _ := result.(SearchResponse)
		if len(response.Results) != 2 {
			t.Errorf("expected 2 results, got %d", len(response.Results))
		}
	})

	t.Run("no results found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tavilyResponse{Results: []tavilyResult{}})
		}))
		defer server.Close()

		tool := WebSearchTool{
			client:  server.Client(),
			apiKey:  "test-key",
			baseURL: server.URL,
		}

		result, err := tool.Execute(context.Background(), map[string]any{"query": "nothing"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		msg, ok := result.(string)
		if !ok {
			t.Fatalf("expected string message for no results, got %T", result)
		}
		if !strings.Contains(msg, "No results found") {
			t.Errorf("unexpected message: %q", msg)
		}
	})

	t.Run("missing query argument", func(t *testing.T) {
		tool := WebSearchTool{}
		_, err := tool.Execute(context.Background(), map[string]any{})
		if err == nil {
			t.Fatal("expected error for missing query")
		}
		if !strings.Contains(err.Error(), "missing or invalid 'query'") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
