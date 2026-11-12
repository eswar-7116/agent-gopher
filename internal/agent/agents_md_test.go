package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAgentsMD(t *testing.T) {
	dir := t.TempDir()
	if got := loadAgentsMD(dir, nil); got != "" {
		t.Fatalf("expected empty for missing file, got %q", got)
	}
	path := filepath.Join(dir, agentsFileName)
	os.WriteFile(path, []byte("  \n"), 0o644)
	if got := loadAgentsMD(dir, nil); got != "" {
		t.Fatalf("expected empty for blank file, got %q", got)
	}
	os.WriteFile(path, []byte("use tabs\n"), 0o644)
	if got := loadAgentsMD(dir, nil); !strings.Contains(got, "use tabs") {
		t.Fatalf("missing content: %q", got)
	}
}
