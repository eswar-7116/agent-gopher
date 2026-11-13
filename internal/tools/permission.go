package tools

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

const (
	PermShell = "shell"
	PermWrite = "write"
)

// What a tool asks the user to approve
type PermissionRequest struct {
	Kind       string
	Command    string // shell: full command
	Background bool   // shell: runs in background
	Path       string // write: target file
	Diff       string // write: unified diff of the change
}

var (
	UseStdinForPrompt = false // used for testing

	// Returns "y", "a" or "n" plus an optional alternative instruction on denial
	PromptPermission func(req PermissionRequest) (choice, alt string)
)

func askPermission(req PermissionRequest) (choice, alt string, err error) {
	if PromptPermission != nil {
		choice, alt = PromptPermission(req)
		return choice, alt, nil
	}

	var reader io.Reader
	if UseStdinForPrompt {
		reader = os.Stdin
	} else {
		tty, err := os.Open(ttyPath)
		if err != nil {
			return "", "", fmt.Errorf("failed to open tty: %v", err)
		}
		defer tty.Close()
		reader = tty
	}

	if req.Kind == PermWrite {
		fmt.Printf("%s\nAgent wants to write %s. [y]es / [N]o / [a]llow all writes this session: ", req.Diff, req.Path)
	} else {
		kind := "command"
		if req.Background {
			kind = "background command"
		}
		fmt.Printf("Agent wants to execute %s %q in your shell. [y]es / [N]o / [a]lways allow: ", kind, req.Command)
	}

	sc := bufio.NewScanner(reader)
	var answer string
	if sc.Scan() {
		answer = strings.TrimSpace(strings.ToLower(sc.Text()))
	}
	switch answer {
	case "a", "always":
		return "a", "", nil
	case "y", "yes":
		return "y", "", nil
	}
	fmt.Print("Denied. What should the agent do instead? (Press Enter to skip): ")
	if sc.Scan() {
		alt = strings.TrimSpace(sc.Text())
	}
	return "n", alt, nil
}

// Builds the error returned when the user denies a request
func deniedError(what, alt string) error {
	if alt != "" {
		return fmt.Errorf("user denied permission for %s. The following instruction was provided by the user as an alternative: %s", what, alt)
	}
	return fmt.Errorf("user denied permission for %s", what)
}

type Whitelist struct {
	mu    sync.RWMutex
	cmds  map[string]bool
	onAdd func(string)
}

func NewWhitelist(cmds []string, onAdd func(string)) *Whitelist {
	w := &Whitelist{cmds: make(map[string]bool, len(cmds)), onAdd: onAdd}
	for _, c := range cmds {
		w.cmds[c] = true
	}
	return w
}

func (w *Whitelist) Allows(cmd string) bool {
	if w == nil {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return isWhitelisted(cmd, w.cmds)
}

func (w *Whitelist) Add(cmd string) {
	if w == nil {
		return
	}
	prefix := extractPrefix(cmd)
	if prefix == "" {
		return
	}
	w.mu.Lock()
	if w.cmds[prefix] {
		w.mu.Unlock()
		return
	}
	w.cmds[prefix] = true
	w.mu.Unlock()
	if w.onAdd != nil {
		w.onAdd(prefix)
	}
}

// Gates file writes (shared by write_file and edit_file)
type WriteGate struct {
	mu       sync.Mutex
	allowAll bool
}

func NewWriteGate(permissive bool) *WriteGate {
	return &WriteGate{allowAll: permissive}
}

// Asks before writing
func (g *WriteGate) Approve(path, diff string) error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.allowAll {
		return nil
	}
	choice, alt, err := askPermission(PermissionRequest{Kind: PermWrite, Path: path, Diff: diff})
	if err != nil {
		return err
	}
	switch choice {
	case "a":
		g.allowAll = true
	case "y":
	default:
		return deniedError("writing "+path, alt)
	}
	return nil
}
