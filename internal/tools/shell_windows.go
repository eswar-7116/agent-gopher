//go:build windows

package tools

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// The console input device used for interactive permission prompts.
const ttyPath = "CONIN$"

func shellPath() string {
	if s := os.Getenv("ComSpec"); s != "" {
		return s
	}
	return "cmd.exe"
}

// Reports the shell used to run commands
func shellName() string {
	return "cmd.exe"
}

// Builds a command that runs cmdStr through cmd.exe
func shellCommand(ctx context.Context, cmdStr string) *exec.Cmd {
	shell := shellPath()
	cmd := exec.CommandContext(ctx, shell)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: `"` + shell + `" /d /s /c "` + cmdStr + `"`,
	}
	cmd.Cancel = func() error { return killTree(cmd) }
	return cmd
}

// Starts cmd in a new process group so cancelling it kills the whole tree.
func detach(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
	cmd.Cancel = func() error { return killTree(cmd) }
}

// Terminates the process and all of its descendants.
func killTree(cmd *exec.Cmd) error {
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	if err := kill.Run(); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}
