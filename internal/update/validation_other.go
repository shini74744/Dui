//go:build !linux

package update

import "os/exec"

// Kernel WireGuard TUN initialization is Linux-only.
func isolateValidation(cmd *exec.Cmd) {}
