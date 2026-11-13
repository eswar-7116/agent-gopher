package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
)

// Builds a realistic-sized conversation: user/assistant turns plus tool
// calls and their output, which is what updateContextEstimate re-marshals
// on every single agent step.
func syntheticConversation(turns int) []json.RawMessage {
	raw := make([]json.RawMessage, 0, turns*3)
	msg := func(m openai.ChatCompletionMessageParamUnion) json.RawMessage {
		b, _ := json.Marshal(m)
		return b
	}
	for i := 0; i < turns; i++ {
		raw = append(raw, msg(openai.UserMessage(strings.Repeat("please fix this bug in the handler ", 4))))
		raw = append(raw, msg(openai.AssistantMessage(strings.Repeat("I'll look at the file and patch it. ", 6))))
		raw = append(raw, msg(openai.ToolMessage(strings.Repeat("line 1\nline 2\nline 3\n", 20), "call_"+strings.Repeat("x", 4))))
	}
	return raw
}

// updateContextEstimate runs on every agent step regardless of tool/model
// cost, so its scaling with conversation length matters.
func BenchmarkContextEstimate_50Turns(b *testing.B) {
	benchmarkContextEstimate(b, 50)
}

func BenchmarkContextEstimate_200Turns(b *testing.B) {
	benchmarkContextEstimate(b, 200)
}

func benchmarkContextEstimate(b *testing.B, turns int) {
	a := NewAgent(AgentOptions{Model: "bench"})
	if err := a.Restore(syntheticConversation(turns)); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		a.updateContextEstimate()
	}
}
