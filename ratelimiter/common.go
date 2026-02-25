package ratelimiter

func normalizeKey(key string) string {
	if key == "" {
		return "unknown"
	}
	return key
}
