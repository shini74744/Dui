package job

import (
	"x-ui/logger"
	"x-ui/web/service"
)

// Period identifies the automatic inbound traffic reset cadence.
type Period string

const (
	Daily   Period = "daily"
	Weekly  Period = "weekly"
	Monthly Period = "monthly"
)

type PeriodicTrafficResetJob struct {
	inboundService service.InboundService
	period         Period
}

func NewPeriodicTrafficResetJob(period Period) *PeriodicTrafficResetJob {
	return &PeriodicTrafficResetJob{period: period}
}

func (j *PeriodicTrafficResetJob) Run() {
	inbounds, err := j.inboundService.GetInboundsByTrafficReset(string(j.period))
	if err != nil {
		logger.Warning("Failed to get inbounds for traffic reset:", err)
		return
	}
	if len(inbounds) == 0 {
		return
	}

	logger.Infof("Running periodic traffic reset job for period: %s", j.period)
	resetCount := 0
	for _, inbound := range inbounds {
		// Reset both the inbound aggregate counters and its client counters.
		// Keeping this scoped by inbound avoids changing unrelated inbounds.
		if err := j.inboundService.ResetInboundTraffics(inbound.Id); err != nil {
			logger.Warning("Failed to reset inbound traffic", inbound.Id, ":", err)
			continue
		}
		if err := j.inboundService.ResetAllClientTraffics(inbound.Id); err != nil {
			logger.Warning("Failed to reset client traffic for inbound", inbound.Id, ":", err)
			continue
		}
		resetCount++
		logger.Infof("Reset traffic for inbound %d (%s)", inbound.Id, inbound.Remark)
	}
	if resetCount > 0 {
		logger.Infof("Periodic traffic reset completed: %d inbounds reset", resetCount)
	}
}
