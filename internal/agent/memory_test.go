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
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func TestMemoryReachesModel(t *testing.T) {
	var lastMsgs []any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		lastMsgs = body["messages"].([]any)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"x","object":"chat.completion","model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer srv.Close()

	wd := t.TempDir()
	t.Chdir(wd)
	t.Setenv("HOME", wd)
	globalDir := memory.GlobalDir()
	projectDir := memory.ProjectDir(wd)
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
