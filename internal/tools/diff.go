package tools

import (
	"strings"

	"github.com/aymanbagabas/go-udiff"
)

// Returns a unified diff
func UnifiedDiff(path, before, after string) string {
	return strings.TrimSuffix(udiff.Unified("a/"+path, "b/"+path, before, after), "\n")
}

// Counts added and removed lines in a unified diff
func DiffStat(diff string) (added, removed int) {
	for line := range strings.SplitSeq(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			removed++
		}
	}
	return added, removed
}
