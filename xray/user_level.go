package xray

func apiUserLevel(v any) uint32 {
	switch n := v.(type) {
	case uint32:
		return n
	case int:
		if n >= 0 && uint64(n) <= 0xffffffff {
			return uint32(n)
		}
	case float64:
		if n >= 0 && n <= 0xffffffff && n == float64(uint32(n)) {
			return uint32(n)
		}
	}
	return 0
}
