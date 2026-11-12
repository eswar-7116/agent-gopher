package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eswar-7116/agent-gopher/internal/memory"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

const (
	RootDirName = ".agent-memory"
	Ext         = ".json"

	idAlphabet = "abcdefghijklmnopqrstuvwxyz1234567890"
	idLength   = 16
)

type Entry struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type State struct {
	ID          string    `json:"id"`
	Version     int       `json:"version"`
	ProjectPath string    `json:"project_path"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Model       string    `json:"model,omitempty"`
	Mode        string    `json:"mode,omitempty"`

	// Entries is the rendered transcript, in display order.
	Entries []Entry `json:"entries,omitempty"`
	// InputHistory is what the user typed, in submission order.
	InputHistory []string `json:"input_history,omitempty"`
	// Messages is the agent's raw conversation history as JSON, kept opaque so
	// that a change in the upstream message types cannot break loading.
	Messages json.RawMessage `json:"messages,omitempty"`
}

// Schema version written into new state files
const Version = 1

// Returns a fresh session id
func NewID() (string, error) {
	id, err := gonanoid.Generate(idAlphabet, idLength)
	if err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return id, nil
}

func Root() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, RootDirName)
}

func Dir(projectPath string) string {
	root := Root()
	if root == "" {
		return ""
	}
	return filepath.Join(root, memory.NormalizeProjectPath(projectPath))
}

// Returns the file a session's state is stored in
func Path(projectPath, id string) string {
	dir := Dir(projectPath)
	if dir == "" || id == "" {
		return ""
	}
	return filepath.Join(dir, id+Ext)
}

// Writes the state to its file
func Save(s *State) (string, error) {
	if s == nil {
		return "", errors.New("nil session state")
	}
	if s.ID == "" {
		return "", errors.New("session has no id")
	}
	dir := Dir(s.ProjectPath)
	if dir == "" {
		return "", errors.New("cannot locate home directory for session storage")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create session dir: %w", err)
	}

	if s.Version == 0 {
		s.Version = Version
	}
	now := time.Now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	s.UpdatedAt = now

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode session: %w", err)
	}
	data = append(data, '\n')

	path := filepath.Join(dir, s.ID+Ext)
	tmp, err := os.CreateTemp(dir, s.ID+".*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp session file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", fmt.Errorf("write session: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("write session: %w", err)
	}
	// Windows will not rename onto an existing file.
	if err := os.Rename(tmpName, path); err != nil {
		if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
			return "", fmt.Errorf("replace session file: %w", err)
		}
		if err := os.Rename(tmpName, path); err != nil {
			return "", fmt.Errorf("replace session file: %w", err)
		}
	}
	return path, nil
}

// Reads a session state from a file path.
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("decode session %s: %w", filepath.Base(path), err)
	}
	return &s, nil
}

// Returns a project's saved sessions, most recently updated first.
func List(projectPath string) ([]*State, error) {
	dir := Dir(projectPath)
	if dir == "" {
		return nil, errors.New("cannot locate home directory for session storage")
	}
	names, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*State
	for _, e := range names {
		if e.IsDir() || !strings.HasSuffix(e.Name(), Ext) {
			continue
		}
		s, err := Load(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
