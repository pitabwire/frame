package security

import (
	"context"

	"github.com/pitabwire/frame/client"
	"github.com/pitabwire/frame/config"
)

const NamespaceProfile = "default/profile"

// ObjectRef represents a reference to an object (resource).
type ObjectRef struct {
	Namespace string // "room", "message", "profile"
	ID        string // Object identifier
}

// SubjectRef represents a reference to a subject (actor).
type SubjectRef struct {
	Namespace string // Usually "profile"
	ID        string // Profile ID
	Relation  string // Optional: for subject sets (e.g., "room:123#member")
}

// RelationTuple represents a relationship between object and subject.
type RelationTuple struct {
	Object   ObjectRef
	Relation string // "owner", "admin", "member", "sender", etc.
	Subject  SubjectRef
}

// CheckRequest represents a permission check request.
type CheckRequest struct {
	Object     ObjectRef
	Permission string // "view", "delete", "send_message", etc.
	Subject    SubjectRef
}

// CheckResult represents the result of a permission check.
type CheckResult struct {
	Allowed   bool
	Reason    string // Explanation for audit
	CheckedAt int64  // Unix timestamp
}

// AuthzOptions contains configuration for authorization.
type AuthzOptions struct {
	Cfg     config.ConfigurationAuthorization
	Client  client.Manager
	Auditor AuditLogger
}

type AuthzOption func(ctx context.Context, opts *AuthzOptions)

// WithAuthorizationConfig adds configuration to existing AuthzOptions.
func WithAuthorizationConfig(cfg config.ConfigurationAuthorization) AuthzOption {
	return func(_ context.Context, opts *AuthzOptions) {
		opts.Cfg = cfg
	}
}

// WithClient adds client for external calls to existing AuthzOptions.
func WithClient(cli client.Manager) AuthzOption {
	return func(_ context.Context, opts *AuthzOptions) {
		opts.Client = cli
	}
}

// WithAuditLogger adds an auditor instance to existing AuthzOptions.
func WithAuditLogger(auditLogger AuditLogger) AuthzOption {
	return func(_ context.Context, opts *AuthzOptions) {
		opts.Auditor = auditLogger
	}
}
