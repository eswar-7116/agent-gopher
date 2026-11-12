//go:build !windows

package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// The controlling terminal used for interactive permission prompts
const ttyPath = "/dev/tty"

func shellPath() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/sh"
}

// Reports the shell used to run commands
func shellName() string {
	return filepath.Base(shellPath())
}

// Builds a command that runs cmdStr through the user's shell.
func shellCommand(ctx context.Context, cmdStr string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, shellPath(), "-c", cmdStr)
	cmd.Cancel = func() error { return cmd.Process.Kill() }
	return cmd
}

// Places cmd in its own process group so cancelling it kills the whole tree.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
