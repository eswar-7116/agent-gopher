package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/eswar-7116/agent-gopher/internal/tools"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func TestToolErrorKeepsOutput(t *testing.T) {
	a := &Agent{
		logger: log.New(io.Discard, "", 0),
		registry: map[string]tools.Tool{
			"t": mockTool{name: "t", execute: func(ctx context.Context, args map[string]any) (any, error) {
				return "compile error: x.go:3: undefined: y", errors.New("exit status 1")
			}},
		},
	}
	got := a.executeToolCall(t.Context(), "t", `{}`)
	if !strings.Contains(got, "exit status 1") || !strings.Contains(got, "undefined: y") {
		t.Errorf("result = %q, want error and output", got)
	}
}

func TestCapOutput(t *testing.T) {
	s := strings.Repeat("a", maxToolOutput) + strings.Repeat("b", 1000)
	got := capOutput(s)
	if len(got) > maxToolOutput+200 || !strings.Contains(got, "1000 characters truncated") {
		t.Errorf("capped output len %d, marker missing or too long", len(got))
	}
	if !strings.HasPrefix(got, "aaa") || !strings.HasSuffix(got, "bbb") {
		t.Error("expected head and tail to be kept")
	}
	if capOutput("short") != "short" {
		t.Error("short output changed")
	}
}

func TestPlanModeShellIsReadOnly(t *testing.T) {
	if err := checkPlanShell(ModePlan, "shell_cmd", map[string]any{"cmd": "ls -la"}); err != nil {
		t.Errorf("ls refused: %v", err)
	}
	if err := checkPlanShell(ModePlan, "shell_cmd", map[string]any{"cmd": "rm -rf x"}); err == nil {
		t.Error("rm allowed in plan mode")
	}
	if err := checkPlanShell(ModePlan, "shell_cmd", map[string]any{"cmd": "ls", "background": true}); err == nil {
		t.Error("background allowed in plan mode")
	}
	if err := checkPlanShell(ModeNormal, "shell_cmd", map[string]any{"cmd": "rm -rf x"}); err != nil {
		t.Errorf("normal mode check: %v", err)
	}
	if !toolAllowed(ModePlan, "shell_cmd") || toolAllowed(ModePlan, "write_file") || toolAllowed(ModePlan, "edit_file") {
		t.Error("plan mode tool allowlist wrong")
	}
}

func TestBasePromptIsFirst(t *testing.T) {
	client := openai.NewClient(option.WithAPIKey("k"))
	ag := NewAgent(AgentOptions{Client: &client, Model: "m", Logger: log.New(io.Discard, "", 0)})
	if !strings.HasPrefix(ag.systemPrompt, basePrompt) {
		t.Errorf("system prompt = %q", ag.systemPrompt)
	}
}

func TestCompactReplacesHistory(t *testing.T) {
	var mu sync.Mutex
	var summarizeReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		msgs := body["messages"].([]any)
		if c, _ := msgs[0].(map[string]any)["content"].(string); c == compactPrompt {
			mu.Lock()
			summarizeReq = body
			mu.Unlock()
			writeCompletion(w, `{"role":"assistant","content":"SUMMARY"}`)
			return
		}
		writeCompletion(w, `{"role":"assistant","content":"reply"}`)
	}))
	defer srv.Close()

	client := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL(srv.URL))
	ag := NewAgent(AgentOptions{Client: &client, Model: "m", Logger: log.New(io.Discard, "", 0)})
	var notices []string
	ag.SetNoticeCallback(func(s string) { notices = append(notices, s) })

	if err := ag.Compact(t.Context()); !errors.Is(err, ErrNothingToCompact) {
		t.Fatalf("empty compact err = %v", err)
	}
	if err := ag.Run(t.Context(), "first question"); err != nil {
		t.Fatal(err)
	}
	if err := ag.Compact(t.Context()); err != nil {
		t.Fatal(err)
	}

	if summarizeReq == nil || !strings.Contains(summarizeReq["messages"].([]any)[1].(map[string]any)["content"].(string), "first question") {
		t.Fatalf("summarizer did not see the history: %v", summarizeReq)
	}
	if _, ok := summarizeReq["tools"]; ok {
		t.Error("summarize request should not include tools")
	}
	msgs := ag.GetMessages()
	last := msgs[len(msgs)-1]
	if last.OfUser == nil || !strings.Contains(last.OfUser.Content.OfString.Value, "SUMMARY") {
		t.Fatalf("last message is not the summary: %+v", last)
	}
	for _, m := range msgs {
		if m.OfAssistant != nil {
			t.Fatal("old assistant messages survived compaction")
		}
	}
	if len(notices) != 1 {
		t.Errorf("notices = %v", notices)
	}

	// A stale epoch makes LogTail return everything
	raw, epoch, err := ag.LogTail(0, len(msgs))
	if err != nil || epoch != 1 || len(raw) != len(msgs) {
		t.Errorf("LogTail after compaction: n=%d epoch=%d err=%v", len(raw), epoch, err)
	}
	if raw, _, _ := ag.LogTail(1, len(msgs)); len(raw) != 0 {
		t.Errorf("LogTail with current epoch returned %d messages", len(raw))
	}
}

func TestAutoCompactWhenWindowFills(t *testing.T) {
	compactions := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if c, _ := body["messages"].([]any)[0].(map[string]any)["content"].(string); c == compactPrompt {
			compactions++
			writeCompletion(w, `{"role":"assistant","content":"S"}`)
			return
		}
		writeCompletion(w, `{"role":"assistant","content":"`+strings.Repeat("z", 400)+`"}`)
	}))
	defer srv.Close()

	client := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL(srv.URL))
	ag := NewAgent(AgentOptions{Client: &client, Model: "m", Logger: log.New(io.Discard, "", 0), ContextWindow: 100})
	for range 2 {
		if err := ag.Run(t.Context(), "q"); err != nil {
			t.Fatal(err)
		}
	}
	if compactions == 0 {
		t.Error("expected automatic compaction")
	}
	if ag.ContextTokens() == 0 {
		t.Error("context tokens not tracked")
	}
}
