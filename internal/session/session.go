package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eswar-7116/agent-gopher/internal/paths"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

const (
	Ext = ".jsonl"

	// Schema version written into new logs
	Version = 1

	idAlphabet = "abcdefghijklmnopqrstuvwxyz1234567890"
	idLength   = 16

	// Max size of one record, since tool results can be large
	maxRecordBytes = 64 << 20 // 64 MiB
)

// Record types
const (
	recMeta    = "meta"    // first line, written once
	recEntry   = "entry"   // transcript line
	recInput   = "input"   // user submission
	recMessage = "message" // agent conversation message
	recEnd     = "end"     // clean shutdown
)

type Entry struct {
	Role    string    `json:"role"`
	Content string    `json:"content"`
	At      time.Time `json:"at,omitzero"`
}

type State struct {
	ID          string
	Version     int
	ProjectPath string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Model       string
	Mode        string

	// Rendered transcript, in display order
	Entries []Entry
	// What the user typed, in submission order
	InputHistory []string
	// Agent conversation, one raw JSON message each
	Messages []json.RawMessage
	// Whether the log ended with a clean shutdown
	Complete bool
}

// One line of the log; every field but Type is optional
type record struct {
	Type string    `json:"type"`
	At   time.Time `json:"at,omitzero"`

	// meta
	ID          string     `json:"id,omitempty"`
	Version     int        `json:"version,omitempty"`
	ProjectPath string     `json:"project_path,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	Model       string     `json:"model,omitempty"`
	Mode        string     `json:"mode,omitempty"`

	// entry
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`

	// input
	Text string `json:"text,omitempty"`

	// message
	Message json.RawMessage `json:"message,omitempty"`
}

type Info struct {
	ID          string
	Path        string
	ProjectPath string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Model       string
	Mode        string
}

func NewID() (string, error) {
	id, err := gonanoid.Generate(idAlphabet, idLength)
	if err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return id, nil
}

// Returns the log file of a session in the current project
func Path(id string) string {
	if paths.SessionsDir == "" || id == "" {
		return ""
	}
	return filepath.Join(paths.SessionsDir, id+Ext)
}

// Returns a line scanner sized for large records
func scanner(f *os.File) *bufio.Scanner {
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxRecordBytes)
	return sc
}

// Folds a session log back into a State, skipping torn lines
func Load(path string) (*State, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	s := &State{}
	sc := scanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var r record
		if err := json.Unmarshal(line, &r); err != nil {
			continue
		}
		s.apply(r)
	}
	if err := sc.Err(); err != nil {
		// Return what was read along with the error
		return s, fmt.Errorf("read session %s: %w", filepath.Base(path), err)
	}
	if s.ID == "" {
		s.ID = strings.TrimSuffix(filepath.Base(path), Ext)
	}
	return s, nil
}

// Folds one record into the state
func (s *State) apply(r record) {
	if !r.At.IsZero() && r.At.After(s.UpdatedAt) {
		s.UpdatedAt = r.At
	}
	switch r.Type {
	case recMeta:
		s.ID, s.Version = r.ID, r.Version
		s.ProjectPath, s.Model = r.ProjectPath, r.Model
		if r.Mode != "" {
			s.Mode = r.Mode
		}
		if r.CreatedAt != nil {
			s.CreatedAt = *r.CreatedAt
		}
	case recEntry:
		s.Entries = append(s.Entries, Entry{Role: r.Role, Content: r.Content, At: r.At})
		s.Complete = false
	case recInput:
		s.InputHistory = append(s.InputHistory, r.Text)
		s.Complete = false
	case recMessage:
		s.Messages = append(s.Messages, r.Message)
		s.Complete = false
	case recEnd:
		s.Complete = true
		if r.Mode != "" {
			s.Mode = r.Mode
		}
	}
}

// Reads just enough of a log to summarize it
func peek(path string) (*Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info := &Info{
		ID:   strings.TrimSuffix(filepath.Base(path), Ext),
		Path: path,
	}
	if st, err := f.Stat(); err == nil {
		info.UpdatedAt = st.ModTime()
	}
	sc := scanner(f)
	for sc.Scan() {
		var r record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil || r.Type != recMeta {
			continue
		}
		if r.ID != "" {
			info.ID = r.ID
		}
		info.ProjectPath, info.Model, info.Mode = r.ProjectPath, r.Model, r.Mode
		if r.CreatedAt != nil {
			info.CreatedAt = *r.CreatedAt
		}
		return info, nil
	}
	return info, nil
}

// Resolves a full or prefix session ID to a log path, or an error if missing/ambiguous
func FindByPrefix(prefix string) (string, error) {
	dir := paths.SessionsDir
	if dir == "" {
		return "", errors.New("session storage is unavailable")
	}
	exact := filepath.Join(dir, prefix+Ext)
	if _, err := os.Stat(exact); err == nil {
		return exact, nil
	}
	names, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var matches []string
	for _, e := range names {
		if e.IsDir() || !strings.HasSuffix(e.Name(), Ext) {
			continue
		}
		id := strings.TrimSuffix(e.Name(), Ext)
		if strings.HasPrefix(id, prefix) {
			matches = append(matches, filepath.Join(dir, e.Name()))
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no session matching %q", prefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous session prefix %q (%d matches)", prefix, len(matches))
	}
}

// Summarizes the current project's sessions, newest first
func List() ([]*Info, error) {
	dir := paths.SessionsDir
	if dir == "" {
		return nil, errors.New("session storage is unavailable")
	}
	names, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Info
	for _, e := range names {
		if e.IsDir() || !strings.HasSuffix(e.Name(), Ext) {
			continue
		}
		info, err := peek(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
