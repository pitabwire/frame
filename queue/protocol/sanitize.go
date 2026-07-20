package protocol

// IsClaimKey reports whether key is claim-shaped authentication metadata.
func IsClaimKey(key string) bool {
	switch key {
	case "sub", "tenant_id", "partition_id", "profile_id",
		"access_id", "contact_id", "device_id", "roles", "service_name":
		return true
	default:
		return false
	}
}

// SanitizeInbound applies trust policy to inbound metadata before processDelivery.
// When trustClaims is false, claim-shaped keys are dropped.
func SanitizeInbound(md map[string]string, trustClaims bool) map[string]string {
	if md == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(md))
	for k, v := range md {
		if !trustClaims && IsClaimKey(k) {
			continue
		}
		out[k] = v
	}
	return out
}

// Allowed raw-header keys when building candidate metadata (defense in depth).
func isAllowedRawHeader(key string) bool {
	switch key {
	case metaTraceParent, metaTraceState, metaBaggage, metaContentType, metaLang:
		return true
	}
	lk := stringsToLower(key)
	if len(lk) >= 8 && lk[:8] == "x-frame-" {
		return true
	}
	return lk == metaFrameEvent
}

func stringsToLower(s string) string {
	b := make([]byte, len(s))
	for i := range len(s) {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
