//go:build !linux

package service

import "os/exec"

func speedWorkerParentDeath(cmd *exec.Cmd) {}
