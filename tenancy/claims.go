package tenancy

// Claims is the storage-layer view of a principal's tenancy. Treat as
// immutable: every transformation returns a new instance.
type Claims struct {
	// TenantID is the single tenant this principal belongs to.
	TenantID string

	// PartitionIDs are every partition this principal can access. One
	// principal may legitimately span multiple partitions (e.g., an
	// operator with access to several branches, an analyst aggregating
	// across groups). Single-partition principals carry one element.
	PartitionIDs []string

	// AccessID is an optional access-control hint propagated through
	// queue metadata and lifecycle hooks.
	AccessID string

	// Skip is true for internal/system callers that should bypass
	// tenancy enforcement. Providers honour Skip by performing no
	// session binding for the conn — the database-side policy's
	// empty-match-all branch then keeps every row visible.
	Skip bool
}

// IsEmpty reports whether the claims carry enforceable tenancy. Empty
// claims behave identically to "no claims attached" from a provider's
// perspective.
func (c *Claims) IsEmpty() bool {
	if c == nil {
		return true
	}
	return c.TenantID == "" && len(c.PartitionIDs) == 0
}

// ExtendPartitions returns a new Claims with the supplied partition IDs
// merged in. Preserves TenantID, AccessID, and Skip unchanged. Empty
// strings are ignored; duplicates are removed; existing order is kept
// and new IDs appended after.
func (c *Claims) ExtendPartitions(partitionIDs ...string) *Claims {
	if c == nil {
		return &Claims{PartitionIDs: dedupedNonEmpty(partitionIDs)}
	}

	merged := make([]string, 0, len(c.PartitionIDs)+len(partitionIDs))
	seen := make(map[string]struct{}, cap(merged))
	for _, p := range c.PartitionIDs {
		if p == "" {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		merged = append(merged, p)
	}
	for _, p := range partitionIDs {
		if p == "" {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		merged = append(merged, p)
	}

	return &Claims{
		TenantID:     c.TenantID,
		PartitionIDs: merged,
		AccessID:     c.AccessID,
		Skip:         c.Skip,
	}
}

func dedupedNonEmpty(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
