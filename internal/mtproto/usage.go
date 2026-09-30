package mtproto

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func usagePathForID(id int) string { return fmt.Sprintf("%s/mtg-%d-usage.json", configDir(), id) }

// The DB is authoritative across helper/panel restarts and offline quota resets.
// mtg owns this file while running; seed it only after the old process stopped.
func writeUsageState(inst Instance) error {
	usage := make(map[string]map[string]int64, len(inst.Secrets))
	for _, s := range inst.Secrets {
		used := s.UsedBytes
		if used < 0 {
			used = 0
		}
		usage[s.Name] = map[string]int64{"quota_used": used, "period_start": time.Now().Unix()}
	}
	data, err := json.Marshal(usage)
	if err != nil {
		return err
	}
	path := usagePathForID(inst.Id)
	if err = os.WriteFile(path+".new", data, 0600); err != nil {
		return err
	}
	return os.Rename(path+".new", path)
}
