package tui

import "github.com/eswar-7116/agent-gopher/internal/tools"

// Messages sent for the agent goroutine
type (
	textMsg        string
	noticeMsg      string
	toolCallMsg    struct{ name, args string }
	doneMsg        struct{ err error }
	compactDoneMsg struct{ err error }
	injectedMsg    []string
	permReqMsg     struct {
		tools.PermissionRequest
		reply chan permReply
	}
)

type permReply struct{ choice, alt string }
