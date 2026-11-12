package agent

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func TestInjectContinuesRun(t *testing.T) {
	var ag *Agent
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if len(bodies) == 1 {
			if !ag.Inject("also this") {
				t.Error("Inject rejected during run")
			}
			writeCompletion(w, `{"role":"assistant","content":"first"}`)
			return
		}
		writeCompletion(w, `{"role":"assistant","content":"second"}`)
	}))
	defer srv.Close()

	client := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL(srv.URL))
	ag = NewAgent(AgentOptions{Client: &client, Model: "m", Logger: log.New(io.Discard, "", 0)})
	var got []string
	ag.SetInjectedCallback(func(texts []string) { got = append(got, texts...) })

	if err := ag.Run(t.Context(), "hi"); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || !strings.Contains(bodies[1], "also this") {
		t.Fatalf("want 2 requests with injected text in the second, got %d", len(bodies))
	}
	if len(got) != 1 || got[0] != "also this" {
		t.Errorf("callback got %q", got)
	}
	if ag.GetLastAssistantMessage() != "second" {
		t.Errorf("last = %q", ag.GetLastAssistantMessage())
	}
	if ag.Inject("late") {
		t.Error("Inject accepted after run ended")
	}
}
