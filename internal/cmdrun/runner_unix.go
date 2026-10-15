//go:build linux

package cmdrun

import (
	"os/exec"
	"syscall"
)

func configureCmd(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// terminateCmd stops the direct child and its descendants in the child's process group.
func terminateCmd(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	// Setpgid makes the child the leader; PGID equals the child's PID.
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
