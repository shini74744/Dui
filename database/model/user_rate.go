package model

import (
	"fmt"
	"math"
)

const DUIRateLevelFlag uint32 = 1 << 31
const MaxUserRateMbps = 10000

func (c Client) UserRateMbps() float64 {
	if c.SpeedLimitMbps != nil {
		return *c.SpeedLimitMbps
	}
	return float64(c.SpeedLimit) * 1024 * 8 / 1000000
}
func (c Client) ValidateUserRate() error {
	n := c.UserRateMbps()
	if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > MaxUserRateMbps {
		return fmt.Errorf("user speed must be between 0 and %d Mbps", MaxUserRateMbps)
	}
	if n > 0 && c.Email == "" {
		return fmt.Errorf("user speed limit requires an email identifier")
	}
	return nil
}
func (c Client) UserRateLevel() uint32 {
	n := c.UserRateMbps()
	if n <= 0 || c.ValidateUserRate() != nil {
		return 0
	}
	b := uint32(math.Round(n * 1000000 / 8))
	if b == 0 {
		b = 1
	}
	return DUIRateLevelFlag | b
}
