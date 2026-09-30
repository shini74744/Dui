//go:build !linux

package service

// The core provides platform-specific TUN setup and reports startup failures.
func checkTunPermissions() error { return nil }
