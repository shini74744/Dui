package model

type ProbeHistory struct {
	ID       uint64  `gorm:"primaryKey" json:"id"`
	Session  string  `gorm:"uniqueIndex:idx_probe_event,priority:1;size:64" json:"-"`
	Sequence uint64  `gorm:"uniqueIndex:idx_probe_event,priority:2" json:"-"`
	Time     int64   `gorm:"index;index:idx_probe_strategy_time,priority:2" json:"time"`
	Strategy string  `gorm:"index:idx_probe_strategy_time,priority:1;size:24" json:"strategy"`
	Outbound string  `gorm:"index;size:1024" json:"outbound"`
	Status   string  `gorm:"size:32" json:"status"`
	Delay    float64 `json:"delay"`
}
