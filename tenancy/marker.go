// Package tenancy provides the storage-layer view of a principal's
// tenancy (Claims), the Provider abstraction for enforcement, and
// helpers to bind tenancy to a request context. The package is
// intentionally narrow: it depends only on security (for the default
// auth-claims derivation) and gorm.io/gorm.
package tenancy

// Tenanted is the structural interface a model must satisfy to be
// enrolled in tenancy enforcement. data.BaseModel satisfies it; custom
// models that want enrollment can satisfy it explicitly.
//
// The tenancy package never imports the data package — enrollment is
// purely structural, so downstream services can roll their own
// tenanted base type if needed.
type Tenanted interface {
	GetTenantID() string
	GetPartitionID() string
	GetAccessID() string
	SetTenantID(string)
	SetPartitionID(string)
	SetAccessID(string)
}

// Unscoped opts a model out of tenancy enforcement. Implement this
// interface to skip RLS policy installation for the model's table.
// The canonical way to satisfy it is to embed UnscopedMarker:
//
//	type LookupTable struct {
//	    ID string
//	    tenancy.UnscopedMarker
//	}
type Unscoped interface {
	TenancyUnscoped()
}

// UnscopedMarker is an empty struct satisfying Unscoped. Embed it in
// a model to opt out of tenancy enforcement.
type UnscopedMarker struct{}

// TenancyUnscoped implements Unscoped.
func (UnscopedMarker) TenancyUnscoped() {}

var _ Unscoped = UnscopedMarker{}
