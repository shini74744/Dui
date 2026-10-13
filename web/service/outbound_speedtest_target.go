package service

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// The target belongs to this test job; it never changes routing or probe settings.
type SpeedTestTarget struct {
	Provider    string `json:"provider"`
	DownloadURL string `json:"downloadUrl"`
	UploadURL   string `json:"uploadUrl"`
	PingURL     string `json:"pingUrl"`
}

func resolveSpeedTarget(target SpeedTestTarget) (SpeedTestTarget, error) {
	switch target.Provider {
	case "", "cloudflare":
		return SpeedTestTarget{Provider: "cloudflare", DownloadURL: speedTestURL, UploadURL: speedUploadURL, PingURL: speedPingURL}, nil
	case "custom":
	default:
		return SpeedTestTarget{}, errors.New("invalid_target")
	}
	for _, value := range []*string{&target.DownloadURL, &target.UploadURL, &target.PingURL} {
		*value = strings.TrimSpace(*value)
		if *value == "" {
			continue
		}
		u, err := url.Parse(*value)
		if err != nil || len(*value) > 2048 || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" || strings.ContainsAny(*value, "\r\n\t ") {
			return SpeedTestTarget{}, errors.New("invalid_target")
		}
		if port := u.Port(); port != "" {
			n, e := strconv.Atoi(port)
			if e != nil || n < 1 || n > 65535 {
				return SpeedTestTarget{}, errors.New("invalid_target")
			}
		}
	}
	if target.DownloadURL == "" {
		return SpeedTestTarget{}, errors.New("invalid_target")
	}
	return target, nil
}
