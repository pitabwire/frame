package security

import (
	"context"
)

type Oauth2ClientRegistrar interface {
	RegisterForJwt(ctx context.Context, iClientHolder InternalOauth2ClientHolder) error
	RegisterForJwtWithParams(ctx context.Context,
		oauth2ServiceAdminHost string, clientName string, clientID string, clientSecret string,
		scope string, audienceList []string, metadata map[string]string) (map[string]any, error)
	UnRegisterForJwt(ctx context.Context,
		oauth2ServiceAdminHost string, clientID string) error
}

type InternalOauth2ClientHolder interface {
	JwtClient() map[string]any
	SetJwtClient(clientID, clientSecret string, jwtCli map[string]any)
	JwtClientID() string
	JwtClientSecret() string
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
	InternalOauth2ClientHolder
	GetOauth2ClientRegistrar(ctx context.Context) Oauth2ClientRegistrar
	GetAuthenticator(ctx context.Context) Authenticator
	GetAuthorizer(ctx context.Context) Authorizer
}
