//go:build linux

package update

import (
	"os/exec"
	"syscall"
)

func isolateValidation(cmd *exec.Cmd) {
	// Clone the whole child into a new network namespace. Unsharing a single
	// Go thread would leave other threads attached to the business network.
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWNET}
}
