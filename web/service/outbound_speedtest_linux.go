package service

import (
	"os/exec"
	"syscall"
)

func speedWorkerParentDeath(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}
