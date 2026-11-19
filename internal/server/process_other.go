//go:build !unix

package server

import (
	"os"
	"os/exec"
)

func stopJobWithServer(*exec.Cmd) {}

const pageOpenFlags = os.O_RDONLY
