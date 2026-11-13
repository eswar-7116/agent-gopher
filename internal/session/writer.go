package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Appends a session's records to its log, buffering until armed
type Writer struct {
	mu      sync.Mutex
	meta    record
	path    string
	f       *os.File
	armed   bool
	closed  bool
	resumed bool // appending to an existing log, so no meta line
	pending []record
}

// Continues an existing log; armed since the session is already worth keeping
func Reopen(path string) *Writer {
	return &Writer{path: path, armed: true, resumed: true}
}

// Prepares a log; no file is touched until armed and flushed
func NewWriter(s *State) *Writer {
	created := s.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	return &Writer{
		meta: record{
			Type:        recMeta,
			ID:          s.ID,
			Version:     Version,
			ProjectPath: s.ProjectPath,
			CreatedAt:   &created,
			Model:       s.Model,
			Mode:        s.Mode,
		},
		path: Path(s.ID),
	}
}

// Returns the log file
func (w *Writer) Path() string { return w.path }

// Lets the next Flush create the log file
func (w *Writer) Arm() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.armed = true
}

// Reports whether Flush will write
func (w *Writer) Armed() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.armed
}

// Reports whether the log file exists yet
func (w *Writer) Created() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f != nil
}

// Queues a transcript line; zero time means now
func (w *Writer) AppendEntry(role, content string, at time.Time) {
	if at.IsZero() {
		at = time.Now()
	}
	w.queue(record{Type: recEntry, At: at, Role: role, Content: content})
}

// Queues a user submission
func (w *Writer) AppendInput(text string) {
	w.queue(record{Type: recInput, At: time.Now(), Text: text})
}

// Queues an agent message, untimed since it is collected at flush
func (w *Writer) AppendMessage(msg json.RawMessage) {
	w.queue(record{Type: recMessage, Message: msg})
}

// Queues a marker that discards earlier agent messages
func (w *Writer) AppendReset() {
	w.queue(record{Type: recReset, At: time.Now()})
}

func (w *Writer) queue(r record) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.pending = append(w.pending, r)
	}
}

// Appends the queued records once armed
func (w *Writer) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.flushLocked()
}

func (w *Writer) flushLocked() error {
	// Empty queue writes nothing, so a Flush after Close cannot reopen the file
	if !w.armed || len(w.pending) == 0 {
		return nil
	}
	if w.path == "" {
		return errors.New("cannot locate home directory for session storage")
	}
	if w.f == nil {
		if err := w.open(); err != nil {
			return err
		}
	}

	// One write per flush keeps the batch together
	var buf []byte
	for _, r := range w.pending {
		line, err := json.Marshal(r)
		if err != nil {
			return fmt.Errorf("encode session record: %w", err)
		}
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}
	if _, err := w.f.Write(buf); err != nil {
		return fmt.Errorf("append session: %w", err)
	}
	// Sync per flush is cheap since flushes happen once per turn
	if err := w.f.Sync(); err != nil {
		return fmt.Errorf("sync session: %w", err)
	}
	w.pending = w.pending[:0]
	return nil
}

// Creates the log and writes its meta line
func (w *Writer) open() error {
	if err := os.MkdirAll(filepath.Dir(w.path), 0o700); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open session: %w", err)
	}
	if w.resumed {
		if err := terminateTornLine(w.path, f); err != nil {
			f.Close()
			return err
		}
		w.f = f
		return nil
	}
	w.meta.At = time.Now()
	line, err := json.Marshal(w.meta)
	if err != nil {
		f.Close()
		return fmt.Errorf("encode session meta: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return fmt.Errorf("write session meta: %w", err)
	}
	w.f = f
	return nil
}

// Ends a line left unfinished by a crash so appends start on a fresh line
func terminateTornLine(path string, f *os.File) error {
	r, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open session: %w", err)
	}
	defer r.Close()
	st, err := r.Stat()
	if err != nil || st.Size() == 0 {
		return err
	}
	last := make([]byte, 1)
	if _, err := r.ReadAt(last, st.Size()-1); err != nil {
		return fmt.Errorf("read session: %w", err)
	}
	if last[0] != '\n' {
		if _, err := f.Write([]byte{'\n'}); err != nil {
			return fmt.Errorf("append session: %w", err)
		}
	}
	return nil
}

// Flushes, writes the end record and closes; no-op if never armed
func (w *Writer) Close(mode string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if !w.armed {
		w.pending = nil
		return nil
	}
	w.pending = append(w.pending, record{Type: recEnd, At: time.Now(), Mode: mode})
	err := w.flushLocked()
	if w.f != nil {
		if cerr := w.f.Close(); err == nil {
			err = cerr
		}
		w.f = nil
	}
	return err
}
