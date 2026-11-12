package paths

import (
	"path/filepath"
	"testing"
)

func TestInitWithBase(t *testing.T) {
	home := t.TempDir()
	InitWithBase(home, "/home/eswar/MyProj")
	t.Cleanup(func() { InitWithBase("", "") })

	root := filepath.Join(home, configDirName)
	for name, c := range map[string][2]string{
		"ConfigDir":        {ConfigDir, root},
		"ConfigFile":       {ConfigFile, filepath.Join(root, "config.json")},
		"SkillsDir":        {SkillsDir, filepath.Join(root, "skills")},
		"GlobalMemoryDir":  {GlobalMemoryDir, root},
		"ProjectMemoryDir": {ProjectMemoryDir, filepath.Join(root, "home-eswar-myproj")},
		"SessionsDir":      {SessionsDir, filepath.Join(home, sessionsDirName, "home-eswar-myproj")},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}

	InitWithBase(home, "")
	if ProjectMemoryDir != "" || SessionsDir != "" {
		t.Errorf("project dirs without a project = (%q, %q), want empty", ProjectMemoryDir, SessionsDir)
	}
}

func TestNormalizeProjectPath(t *testing.T) {
	for in, want := range map[string]string{
		`C:\Users\eswar\MyProj`: "c-users-eswar-myproj",
		"/home/eswar/MyProj":    "home-eswar-myproj",
		"/":                     "root",
	} {
		if got := NormalizeProjectPath(in); got != want {
			t.Errorf("NormalizeProjectPath(%q) = %q, want %q", in, got, want)
		}
	}
}
