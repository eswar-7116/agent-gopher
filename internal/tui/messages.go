package tui

// Messages sent from the agent goroutine
type (
	textMsg     string
	toolCallMsg struct{ name, args string }
	doneMsg     struct{ err error }
	permReqMsg  struct {
		cmd        string
		background bool
		reply      chan permReply
	}
)

type permReply struct{ choice, alt string }
