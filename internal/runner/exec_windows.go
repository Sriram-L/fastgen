//go:build windows

package runner

import (
	"os/exec"
	"syscall"
)

func processGroupAttr() *syscall.SysProcAttr {
	return nil
}

func processGroupKill(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
