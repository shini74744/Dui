package model

import (
	"encoding/json"
	"math"
	"testing"
)

func TestDUIUserRate(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want uint32
	}{{`{"email":"a","speedLimitMbps":300}`, DUIRateLevelFlag | 37500000}, {`{"email":"a","speedLimit":1024}`, DUIRateLevelFlag | 1048576}, {`{"email":"a","speedLimit":1024,"speedLimitMbps":0}`, 0}} {
		var c Client
		if err := json.Unmarshal([]byte(tc.raw), &c); err != nil {
			t.Fatal(err)
		}
		if err := c.ValidateUserRate(); err != nil {
			t.Fatal(err)
		}
		if c.UserRateLevel() != tc.want {
			t.Fatalf("%s: %d != %d", tc.raw, c.UserRateLevel(), tc.want)
		}
	}
	for _, v := range []float64{-1, 10001, math.Inf(1), math.NaN()} {
		c := Client{Email: "a", SpeedLimitMbps: &v}
		if c.ValidateUserRate() == nil {
			t.Fatal("accepted invalid speed", v)
		}
	}
}
