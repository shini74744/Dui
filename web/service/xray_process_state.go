package service

import (
	"errors"
	"sync"
	"sync/atomic"
	"x-ui/xray"
)

// The slot is stable for the lifetime of the panel. Only fully initialized
// processes are published; concurrent status/traffic readers retain safe access.
type xrayProcessSlot struct {
	atomic.Pointer[xray.Process]
	publication sync.RWMutex
}

func (s *xrayProcessSlot) IsRunning() bool { p := s.Load(); return p != nil && p.IsRunning() }
func (s *xrayProcessSlot) GetAPIPort() int {
	if p := s.Load(); p != nil {
		return p.GetAPIPort()
	}
	return 0
}
func (s *xrayProcessSlot) GetErr() error {
	if p := s.Load(); p != nil {
		return p.GetErr()
	}
	return nil
}
func (s *xrayProcessSlot) GetResult() string {
	if p := s.Load(); p != nil {
		return p.GetResult()
	}
	return ""
}
func (s *xrayProcessSlot) GetVersion() string {
	if p := s.Load(); p != nil {
		return p.GetVersion()
	}
	return "Unknown"
}
func (s *xrayProcessSlot) GetUptime() uint64 {
	if p := s.Load(); p != nil {
		return p.GetUptime()
	}
	return 0
}
func (s *xrayProcessSlot) GetConfig() *xray.Config {
	if p := s.Load(); p != nil {
		return p.GetConfig()
	}
	return nil
}
func (s *xrayProcessSlot) GetOnlineClients() []string {
	if p := s.Load(); p != nil {
		return p.GetOnlineClients()
	}
	return nil
}
func (s *xrayProcessSlot) Stop() error {
	if p := s.Load(); p != nil {
		return p.Stop()
	}
	return errors.New("xray is not running")
}

func (s *xrayProcessSlot) SetOnlineClients(clients []string) {
	if p := s.Load(); p != nil {
		p.SetOnlineClients(clients)
	}
}

// Readers wait for startup to publish its process. Otherwise a just-opened API
// listener could accept panel CRUD before the panel can hot-apply that change.
func (s *xrayProcessSlot) Load() *xray.Process {
	s.publication.RLock()
	defer s.publication.RUnlock()
	return s.Pointer.Load()
}
func (s *xrayProcessSlot) start(next *xray.Process) error {
	s.publication.Lock()
	defer s.publication.Unlock()
	err := next.Start()
	s.Pointer.Store(next)
	return err
}
