package job

import (
	"sync"
	"x-ui/logger"
	"x-ui/web/service"
)

type HelperProtocolJob struct {
	mu   sync.Mutex
	core service.XrayService
}

func NewHelperProtocolJob() *HelperProtocolJob { return &HelperProtocolJob{} }
func (j *HelperProtocolJob) Run() {
	if !j.mu.TryLock() {
		return
	}
	defer j.mu.Unlock()
	if err := j.core.SyncHelperProtocols(); err != nil {
		logger.Warning("helper reconcile: ", err)
	}
}
