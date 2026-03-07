package security

import (
	"context"
	"crypto/tls"
)

type WorkloadAPI interface {
	Setup(ctx context.Context) (*tls.Config, error)
	Close()
}

type Authenticator interface {
	Authenticate(ctx context.Context, jwtToken string, options ...AuthOption) (context.Context, error)
}

// Authorizer is the core authorization service interface.
// Implementations can be swapped without affecting business logic.
type Authorizer interface {
	// Check verifies if a subject has permission on an object.
	Check(ctx context.Context, req CheckRequest) (CheckResult, error)

	// BatchCheck verifies multiple permissions in one call (for efficiency).
	BatchCheck(ctx context.Context, requests []CheckRequest) ([]CheckResult, error)

	// WriteTuple creates a relationship tuple.
	WriteTuple(ctx context.Context, tuple RelationTuple) error

	// WriteTuples creates multiple relationship tuples atomically.
	WriteTuples(ctx context.Context, tuples []RelationTuple) error

	// DeleteTuple removes a relationship tuple.
	DeleteTuple(ctx context.Context, tuple RelationTuple) error

	// DeleteTuples removes multiple relationship tuples atomically.
	DeleteTuples(ctx context.Context, tuples []RelationTuple) error

	// ListRelations returns all relations for an object.
	ListRelations(ctx context.Context, object ObjectRef) ([]RelationTuple, error)

	// ListSubjectRelations returns all objects a subject has relations to.
	ListSubjectRelations(ctx context.Context, subject SubjectRef, namespace string) ([]RelationTuple, error)

	// Expand returns all subjects with a given relation (for member listing).
	Expand(ctx context.Context, object ObjectRef, relation string) ([]SubjectRef, error)
}

// AuditLogger logs authorization decisions for security audit.
type AuditLogger interface {
	LogDecision(ctx context.Context, req CheckRequest, result CheckResult, metadata map[string]string) error
}

type Manager interface {
	GetWorkloadAPI(ctx context.Context) WorkloadAPI
	GetAuthenticator(ctx context.Context) Authenticator
	GetAuthorizer(ctx context.Context) Authorizer
	// Close releases resources held by the security manager and its components.
	Close()
}
