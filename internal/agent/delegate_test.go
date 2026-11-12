package agent

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func TestDelegateTaskRunsSubAgent(t *testing.T) {
	var mu sync.Mutex
	var requests []map[string]any

	// Scripted LLM: parent calls delegate_task, sub-agent answers, parent finishes.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		requests = append(requests, body)
		mu.Unlock()

		msgs := body["messages"].([]any)
		first := msgs[0].(map[string]any)
		var msg string
		switch {
		case first["role"] == "system" && strings.Contains(first["content"].(string), "You are a sub-agent"):
			msg = `{"role":"assistant","content":"sub report"}`
		case msgs[len(msgs)-1].(map[string]any)["role"] == "user": // parent, first turn
			msg = `{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"delegate_task","arguments":"{\"task\":\"do it\"}"}}]}`
		default: // parent, after tool result
			msg = `{"role":"assistant","content":"done"}`
		}
		writeCompletion(w, msg)
	}))
	defer srv.Close()

	client := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL(srv.URL))
	ag := NewAgent(AgentOptions{Client: &client, Model: "m", Logger: log.New(io.Discard, "", 0)})

	var calls []string
	ag.SetToolCallCallback(func(name, _ string) { calls = append(calls, name) })

	if err := ag.Run(t.Context(), "hi"); err != nil {
		t.Fatal(err)
	}
	if got := ag.GetLastAssistantMessage(); got != "done" {
		t.Errorf("parent final message = %q", got)
	}

	// The parent's history must contain only the sub-agent's report, not its internals.
	var toolMsg string
	for _, m := range ag.GetMessages() {
		if m.OfTool != nil {
			toolMsg = m.OfTool.Content.OfString.Value
		}
	}
	if toolMsg != "sub report" {
		t.Errorf("tool result = %q", toolMsg)
	}
	if len(requests) != 3 {
		t.Errorf("expected 3 LLM requests, got %d", len(requests))
	}

	// The sub-agent must not be offered delegate_task.
	subTools, _ := json.Marshal(requests[1]["tools"])
	if strings.Contains(string(subTools), delegateToolName) {
		t.Error("sub-agent was offered delegate_task")
	}
	parentTools, _ := json.Marshal(requests[0]["tools"])
	if !strings.Contains(string(parentTools), delegateToolName) {
		t.Error("parent was not offered delegate_task")
	}
}

func TestDelegateTaskValidatesArgs(t *testing.T) {
	d := delegateTool{}
	if _, err := d.Execute(t.Context(), map[string]any{}); err == nil {
		t.Error("expected error for missing task")
	}
	if _, err := d.Execute(t.Context(), map[string]any{"task": "  "}); err == nil {
		t.Error("expected error for blank task")
	}
}

func TestRunStopsAfterMaxSteps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeCompletion(w, `{"role":"assistant","content":"","tool_calls":[{"id":"c","type":"function","function":{"name":"nope","arguments":"{}"}}]}`)
	}))
	defer srv.Close()

	client := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL(srv.URL))
	ag := NewAgent(AgentOptions{Client: &client, Model: "m", Logger: log.New(io.Discard, "", 0)})
	ag.maxSteps = 3
	if err := ag.Run(t.Context(), "hi"); err == nil {
		t.Error("expected step limit error")
	}
}
