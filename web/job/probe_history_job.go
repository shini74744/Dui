package job

import (
	"sync"
	"x-ui/web/service"
)

type ProbeHistoryJob struct{ mu sync.Mutex }

func NewProbeHistoryJob() *ProbeHistoryJob { return &ProbeHistoryJob{} }
func (j *ProbeHistoryJob) Run() {
	if !j.mu.TryLock() {
		return
	}
	defer j.mu.Unlock()
	service.CollectProbeHistory(service.ProbeHistoryAPIPort())
}
