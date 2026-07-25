package protocol

// ClaimTrustLevel controls which claim-shaped metadata keys are accepted
// when reconstructing auth claims on consumers.
type ClaimTrustLevel int

const (
	// TrustNone drops all claim-shaped keys (push default via SanitizeInbound false).
	TrustNone ClaimTrustLevel = iota
	// TrustTenancyOnly keeps tenant/partition/identity keys but strips roles/service_name
	// so metadata cannot force tenancy.Claims.Skip via roles=internal.
	TrustTenancyOnly
	// TrustAll accepts full ClaimsFromMap input (legacy pull default).
	TrustAll
)

// IsClaimKey reports whether key is claim-shaped authentication metadata.
func IsClaimKey(key string) bool {
	switch key {
	case "sub", "tenant_id", "partition_id", "partition_ids", "profile_id",
		"access_id", "contact_id", "device_id", "roles", "service_name":
		return true
	default:
		return false
	}
}

// isPrivilegeClaimKey keys that can escalate privilege (Skip / internal).
func isPrivilegeClaimKey(key string) bool {
	switch key {
	case "roles", "service_name":
		return true
	default:
		return false
	}
}

// ApplyClaimTrust filters metadata according to trust level.
func ApplyClaimTrust(md map[string]string, level ClaimTrustLevel) map[string]string {
	if md == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(md))
	for k, v := range md {
		switch level {
		case TrustAll:
			out[k] = v
		case TrustNone:
			if IsClaimKey(k) {
				continue
			}
			out[k] = v
		case TrustTenancyOnly:
			if isPrivilegeClaimKey(k) {
				continue
			}
			out[k] = v
		default:
			if IsClaimKey(k) {
				continue
			}
			out[k] = v
		}
	}
	return out
}

// SanitizeInbound applies trust policy to inbound metadata before processDelivery.
// When trustClaims is false, claim-shaped keys are dropped (TrustNone).
// When true, all keys are kept (TrustAll). Prefer ApplyClaimTrust for finer control.
func SanitizeInbound(md map[string]string, trustClaims bool) map[string]string {
	if trustClaims {
		return ApplyClaimTrust(md, TrustAll)
	}
	return ApplyClaimTrust(md, TrustNone)
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
