//go:build !linux

package cmdrun

import "os/exec"

func configureCmd(cmd *exec.Cmd) {}

func terminateCmd(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
