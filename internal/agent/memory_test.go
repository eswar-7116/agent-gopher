package agent

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eswar-7116/agent-gopher/internal/memory"
	"github.com/eswar-7116/agent-gopher/internal/paths"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func TestMemoryReachesModel(t *testing.T) {
	var lastMsgs []any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		lastMsgs = body["messages"].([]any)
		writeCompletion(w, `{"role":"assistant","content":"ok"}`)
	}))
	defer srv.Close()

	wd := t.TempDir()
	t.Chdir(wd)
	paths.InitWithBase(wd, wd)
	t.Cleanup(func() { paths.InitWithBase("", "") })
	globalDir, projectDir := paths.GlobalMemoryDir, paths.ProjectMemoryDir
	if _, err := memory.Add(globalDir, "Uses fish shell"); err != nil {
		t.Fatal(err)
	}
	if _, err := memory.Add(projectDir, "Tests use testify"); err != nil {
		t.Fatal(err)
	}

	client := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL(srv.URL))
	ag := NewAgent(AgentOptions{Client: &client, Model: "m", Logger: log.New(io.Discard, "", 0)})
	if ag.MemoryDir(memory.ScopeGlobal) != globalDir || ag.MemoryDir(memory.ScopeProject) != projectDir {
		t.Fatalf("memory dirs = (%q, %q), want (%q, %q)", ag.MemoryDir(memory.ScopeGlobal), ag.MemoryDir(memory.ScopeProject), globalDir, projectDir)
	}

	systemText := func() string {
		var b strings.Builder
		for _, m := range lastMsgs {
			if m := m.(map[string]any); m["role"] == "system" {
				b.WriteString(m["content"].(string))
				b.WriteString("\n")
			}
		}
		return b.String()
	}

	if err := ag.Run(t.Context(), "hi"); err != nil {
		t.Fatal(err)
	}
	if s := systemText(); !strings.Contains(s, "Tests use testify") || !strings.Contains(s, "Uses fish shell") || !strings.Contains(s, memory.ToolName) {
		t.Errorf("first run missing memory/guidance: %q", s)
	}

	// A run with unchanged memory must not repeat it.
	n := len(lastMsgs)
	if err := ag.Run(t.Context(), "again"); err != nil {
		t.Fatal(err)
	}
	if len(lastMsgs) != n+2 { // + user, + previous assistant reply
		t.Errorf("unchanged memory added messages: %d -> %d", n, len(lastMsgs))
	}

	if _, err := memory.Add(projectDir, "Use Bun for PostgreSQL"); err != nil {
		t.Fatal(err)
	}
	if err := ag.Run(t.Context(), "third"); err != nil {
		t.Fatal(err)
	}
	if s := systemText(); !strings.Contains(s, "Use Bun for PostgreSQL") {
		t.Errorf("updated memory not sent: %q", s)
	}
}

func TestRestoreKeepsToolCallsAndDoesNotRepeatMemory(t *testing.T) {
	var lastMsgs []any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		lastMsgs = body["messages"].([]any)
		writeCompletion(w, `{"role":"assistant","content":"ok"}`)
	}))
	defer srv.Close()

	wd := t.TempDir()
	t.Chdir(wd)
	paths.InitWithBase(wd, wd)
	t.Cleanup(func() { paths.InitWithBase("", "") })
	if _, err := memory.Add(paths.ProjectMemoryDir, "Tests use testify"); err != nil {
		t.Fatal(err)
	}
	client := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL(srv.URL))
	newAgent := func() *Agent {
		return NewAgent(AgentOptions{Client: &client, Model: "m", Logger: log.New(io.Discard, "", 0)})
	}

	first := newAgent()
	if err := first.Run(t.Context(), "hi"); err != nil {
		t.Fatal(err)
	}
	first.appendMessages(
		openai.ChatCompletionMessageParamUnion{OfAssistant: &openai.ChatCompletionAssistantMessageParam{
			ToolCalls: []openai.ChatCompletionMessageToolCallUnionParam{{OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID:       "call_1",
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{Name: "read_file", Arguments: `{"path":"x"}`},
			}}},
		}},
		openai.ToolMessage("contents", "call_1"),
	)
	saved, _, err := first.LogTail(0, 0)
	if err != nil {
		t.Fatal(err)
	}

	resumed := newAgent()
	if err := resumed.Restore(saved); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	again, _, err := resumed.LogTail(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != len(saved) || string(again[len(again)-2]) != string(saved[len(saved)-2]) {
		t.Fatalf("tool call lost in round trip:\n got %s\nwant %s", again[len(again)-2], saved[len(saved)-2])
	}

	if err := resumed.Run(t.Context(), "again"); err != nil {
		t.Fatal(err)
	}
	if len(lastMsgs) != len(saved)+1 {
		t.Fatalf("resumed run with unchanged memory sent %d messages, want %d", len(lastMsgs), len(saved)+1)
	}

	if _, err := memory.Add(paths.ProjectMemoryDir, "Use Bun for PostgreSQL"); err != nil {
		t.Fatal(err)
	}
	changed := newAgent()
	if err := changed.Restore(saved); err != nil {
		t.Fatal(err)
	}
	if err := changed.Run(t.Context(), "again"); err != nil {
		t.Fatal(err)
	}
	if s, _ := json.Marshal(lastMsgs); !strings.Contains(string(s), "Use Bun for PostgreSQL") {
		t.Fatal("memory changed while the session was closed but was not announced")
	}
}
