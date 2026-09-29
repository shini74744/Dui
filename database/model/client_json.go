package model

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Older 3x-ui/DUI saved tgId as a string; keep such clients readable after upgrade.
func (c *Client) UnmarshalJSON(b []byte) error {
	type plain Client
	var decoded plain
	v := struct {
		*plain
		TgID json.RawMessage `json:"tgId"`
	}{plain: &decoded}
	if e := json.Unmarshal(b, &v); e != nil {
		return e
	}
	if len(v.TgID) > 0 && string(v.TgID) != "null" {
		if e := json.Unmarshal(v.TgID, &decoded.TgID); e != nil {
			var s string
			if e = json.Unmarshal(v.TgID, &s); e != nil {
				return e
			}
			s = strings.TrimSpace(s)
			if s != "" {
				n, e := strconv.ParseInt(s, 10, 64)
				if e != nil {
					return e
				}
				decoded.TgID = n
			}
		}
	}
	*c = Client(decoded)
	return nil
}
