package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// Replies as an SSE stream
func writeCompletion(w http.ResponseWriter, msgJSON string) {
	var msg map[string]any
	if err := json.Unmarshal([]byte(msgJSON), &msg); err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	send := func(delta map[string]any, finish any) {
		b, _ := json.Marshal(map[string]any{
			"id": "x", "object": "chat.completion.chunk", "model": "m",
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
		})
		fmt.Fprintf(w, "data: %s\n\n", b)
	}
	send(map[string]any{"role": "assistant"}, nil)
	content, _ := msg["content"].(string)
	for _, r := range content {
		send(map[string]any{"content": string(r)}, nil)
	}
	finish := "stop"
	if calls, ok := msg["tool_calls"].([]any); ok && len(calls) > 0 {
		finish = "tool_calls"
		for i, c := range calls {
			c.(map[string]any)["index"] = i
		}
		send(map[string]any{"tool_calls": calls}, nil)
	}
	send(map[string]any{}, finish)
	io.WriteString(w, "data: [DONE]\n\n")
}

func TestTextIsStreamed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeCompletion(w, `{"role":"assistant","content":"hello"}`)
	}))
	defer srv.Close()

	client := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL(srv.URL))
	ag := NewAgent(AgentOptions{Client: &client, Model: "m", Logger: log.New(io.Discard, "", 0)})

	var deltas []string
	ag.SetTextCallback(func(d string) { deltas = append(deltas, d) })
	if err := ag.Run(t.Context(), "hi"); err != nil {
		t.Fatal(err)
	}
	if len(deltas) < 2 || strings.Join(deltas, "") != "hello" {
		t.Errorf("deltas = %q, want several chunks joining to hello", deltas)
	}
	if got := ag.GetLastAssistantMessage(); got != "hello" {
		t.Errorf("final message = %q", got)
	}
}

func TestCancelKeepsPartialReply(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"id":"x","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":"half"}}]}`+"\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	client := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
	ag := NewAgent(AgentOptions{Client: &client, Model: "m", Logger: log.New(io.Discard, "", 0)})

	ctx, cancel := context.WithCancel(t.Context())
	ag.SetTextCallback(func(string) { cancel() })
	if err := ag.Run(ctx, "hi"); err == nil {
		t.Fatal("want cancellation error")
	}
	if got := ag.GetLastAssistantMessage(); got != "half" {
		t.Errorf("last assistant = %q, want partial reply", got)
	}
}
