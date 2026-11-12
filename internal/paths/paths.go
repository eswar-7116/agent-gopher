// Package paths resolves every on-disk location once at startup.
package paths

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	configDirName   = ".agent-gopher"
	sessionsDirName = ".agent-memory"
)

var (
	ProjectPath string

	ConfigDir  string // ~/.agent-gopher
	ConfigFile string // ~/.agent-gopher/config.json
	SkillsDir  string // ~/.agent-gopher/skills

	GlobalMemoryDir  string // ~/.agent-gopher
	ProjectMemoryDir string // ~/.agent-gopher/<project>

	SessionsDir string // ~/.agent-memory/<project>
)

// Resolves all paths from the user's home and working directories.
func Init() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	wd, _ := os.Getwd()
	InitWithBase(home, wd)
	return nil
}

// Resolves all paths under the given home for the given project.
func InitWithBase(home, projectPath string) {
	ProjectPath = projectPath
	ConfigDir = filepath.Join(home, configDirName)
	ConfigFile = filepath.Join(ConfigDir, "config.json")
	SkillsDir = filepath.Join(ConfigDir, "skills")
	GlobalMemoryDir = ConfigDir
	ProjectMemoryDir, SessionsDir = "", ""
	if projectPath != "" {
		name := NormalizeProjectPath(projectPath)
		ProjectMemoryDir = filepath.Join(ConfigDir, name)
		SessionsDir = filepath.Join(home, sessionsDirName, name)
	}
}

// Flattens a project path into a single directory name.
func NormalizeProjectPath(projectPath string) string {
	cleaned := path.Clean(strings.ReplaceAll(projectPath, `\`, "/"))
	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = strings.ReplaceAll(cleaned, ":", "")
	normalized := strings.ToLower(strings.ReplaceAll(cleaned, "/", "-"))
	if normalized == "" || normalized == "." {
		return "root"
	}
	return normalized
}
