//go:build !linux

package update

import "errors"

func Supported(panel string) bool         { return false }
func Start(j Job) (Job, error)            { return j, errors.New("unsupported_installation") }
func Ready(root string, coreRunning bool) {}
func Run(root, id string) int             { return 1 }

func Status(root string) (Job, error) { return Load(root) }
