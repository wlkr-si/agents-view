//go:build unix

package server

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func stopJobWithServer(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}

const pageOpenFlags = os.O_RDONLY | syscall.O_NONBLOCK
