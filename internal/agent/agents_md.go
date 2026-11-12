package agent

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

const agentsFileName = "AGENTS.md"

// Reads AGENTS.md from dir and returns a system prompt built from it
func loadAgentsMD(dir string, logger *log.Logger) string {
	path := filepath.Join(dir, agentsFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) && logger != nil {
			logger.Printf("failed to read %s: %v", path, err)
		}
		return ""
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return ""
	}
	if logger != nil {
		logger.Printf("loaded %s (%d bytes)", path, len(data))
	}
	return "The following project instructions come from " + agentsFileName +
		" in the working directory. Follow them when relevant.\n\n" + content
}
