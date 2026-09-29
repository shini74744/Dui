package service

import (
	"encoding/json"
	"strings"
	"x-ui/xray"
)

// Normalize retired TLS keys on the generated config, without rewriting saved settings.
func normalizeDUITLS(value any) {
	switch v := value.(type) {
	case map[string]any:
		if tls, ok := v["tlsSettings"].(map[string]any); ok {
			if _, exists := tls["verifyPeerCertByName"]; !exists {
				if names, ok := tls["verifyPeerCertInNames"].([]any); ok {
					var parts []string
					for _, n := range names {
						if s, ok := n.(string); ok {
							parts = append(parts, s)
						}
					}
					tls["verifyPeerCertByName"] = strings.Join(parts, ",")
				}
			}
			delete(tls, "verifyPeerCertInNames")
			delete(tls, "echForceQuery")
		}
		for _, child := range v {
			normalizeDUITLS(child)
		}
	case []any:
		for _, child := range v {
			normalizeDUITLS(child)
		}
	}
}
func applyDUITLSCompatibility(c *xray.Config) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	var v any
	if err = json.Unmarshal(b, &v); err != nil {
		return err
	}
	normalizeDUITLS(v)
	b, err = json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, c)
}
