package tui

import (
	"strings"

	"github.com/eswar-7116/agent-gopher/internal/agent"
	"github.com/eswar-7116/agent-gopher/internal/memory"
)

type slashCmd struct{ name, desc string }

var slashCmds = []slashCmd{
	{"/mode", "Switch mode: /mode plan | normal (no arg toggles)"},
	{"/plan", "Switch to plan mode (read-only)"},
	{"/normal", "Switch to normal mode"},
	{"/gopher", "Memory: /gopher memory [global|project] | remember [global|project] <fact> | forget [global|project] <n|text>"},
	{"/exit", "Quit Agent Gopher"},
	{"/quit", "Quit Agent Gopher"},
}

// Returns slash commands matching the current input prefix.
func (m *Model) suggestions() []slashCmd {
	v := m.input.Value()
	if m.perm != nil || !strings.HasPrefix(v, "/") || strings.ContainsAny(v, " \t") {
		return nil
	}
	var out []slashCmd
	for _, c := range slashCmds {
		if strings.HasPrefix(c.name, v) {
			out = append(out, c)
		}
	}
	return out
}

func (m *Model) runSlash(text string) bool {
	fields := strings.Fields(text)
	var target agent.Mode
	switch fields[0] {
	case "/plan":
		target = agent.ModePlan
	case "/normal":
		target = agent.ModeNormal
	case "/mode":
		if len(fields) < 2 {
			target = agent.ModePlan
			if m.agent.Mode() == agent.ModePlan {
				target = agent.ModeNormal
			}
			break
		}
		var err error
		if target, err = agent.ParseMode(fields[1]); err != nil {
			m.input.Reset()
			m.layout()
			m.add(roleError, err.Error())
			return true
		}
	case "/gopher":
		m.input.Reset()
		m.layout()
		m.runGopher(strings.TrimSpace(strings.TrimPrefix(text, "/gopher")))
		return true
	default:
		return false
	}
	m.input.Reset()
	m.layout()
	m.agent.SetMode(target)
	m.add(roleInfo, "Switched to "+target.String()+" mode.")
	return true
}

const gopherUsage = "Usage: /gopher memory [global|project] · /gopher remember [global|project] <fact> · /gopher forget [global|project] <number|text>"

// Handles "/gopher <subcommand>" project-memory commands.
func (m *Model) runGopher(args string) {
	cmd, rest, _ := strings.Cut(args, " ")
	rest = strings.TrimSpace(rest)
	scope, rest := memoryScope(rest)
	dir := m.agent.MemoryDir(scope)
	if dir == "" {
		m.add(roleError, "Memory is unavailable for the selected scope.")
		return
	}
	switch cmd {
	case "memory":
		entries := memory.List(dir)
		if len(entries) == 0 {
			m.add(roleInfo, "No memories yet. Add one with /gopher remember "+scope+" <fact> ("+dir+").")
			return
		}
		m.add(roleInfo, memory.Format(entries))
	case "remember":
		added, err := memory.Add(dir, rest)
		switch {
		case err != nil:
			m.add(roleError, err.Error())
		case !added:
			m.add(roleInfo, "Already remembered.")
		default:
			m.add(roleInfo, "Remembered ("+scope+").")
		}
	case "forget":
		e, err := memory.Remove(dir, rest)
		if err != nil {
			m.add(roleError, err.Error())
			return
		}
		m.add(roleInfo, "Forgot ("+scope+"): "+e)
	default:
		m.add(roleInfo, gopherUsage)
	}
}

func memoryScope(args string) (string, string) {
	scope := memory.ScopeProject
	name, rest, ok := strings.Cut(args, " ")
	if !ok {
		name, rest = args, ""
	}
	if name == memory.ScopeGlobal || name == memory.ScopeProject {
		return name, strings.TrimSpace(rest)
	}
	return scope, args
}
