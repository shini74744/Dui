package service

import (
	"os"
	"x-ui/internal/mtproto"
	"x-ui/internal/tuic"
)

type HelperCapability struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

func GetHelperCapabilities() map[string]HelperCapability {
	result := map[string]HelperCapability{"amneziawg": {Available: true}}
	for protocol, path := range map[string]string{"tuic": tuic.GetBinaryPath(), "mtproto": mtproto.GetBinaryPath()} {
		st, err := os.Stat(path)
		ready := err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0111 != 0
		capability := HelperCapability{Available: ready}
		if !ready {
			capability.Reason = "当前平台未安装兼容的配套组件，请安装包含该组件的 DUI 发行包。"
		}
		result[protocol] = capability
	}
	return result
}
