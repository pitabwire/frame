package authorizer

import (
	"context"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/security"
)

// ResourceCheckerOption configures a ResourceAccessChecker.
type ResourceCheckerOption func(*ResourceAccessChecker)

// WithResourceSubjectNamespace overrides the default subject namespace.
func WithResourceSubjectNamespace(ns string) ResourceCheckerOption {
	return func(c *ResourceAccessChecker) {
		c.subjectNamespace = ns
	}
}

// WithConstraints adds contextual constraints that are evaluated after the
// Keto relation check passes. If any constraint returns an error, the overall
// check is denied. Use this for time-of-day, location, or other runtime
// conditions that cannot be modelled as Keto relation tuples.
func WithConstraints(constraints ...AccessConstraint) ResourceCheckerOption {
	return func(c *ResourceAccessChecker) {
		c.constraints = append(c.constraints, constraints...)
	}
}

// WithPermissionConstraints adds constraints that apply only when checking
// a specific permission. These are evaluated after global constraints.
func WithPermissionConstraints(permission string, constraints ...AccessConstraint) ResourceCheckerOption {
	return func(c *ResourceAccessChecker) {
		if c.permConstraints == nil {
			c.permConstraints = make(map[string][]AccessConstraint)
		}
		c.permConstraints[permission] = append(c.permConstraints[permission], constraints...)
	}
}

// ResourceAccessChecker verifies per-resource-instance permissions (Plane 3).
// Unlike TenancyAccessChecker and FunctionChecker which derive the object ID
// from claims (tenant/partition), ResourceAccessChecker takes the resource ID
// explicitly because resource instances are not part of the authentication claims.
//
// Example namespaces: chat_room, file, file_version.
type ResourceAccessChecker struct {
	authorizer       security.Authorizer
	objectNamespace  string
	subjectNamespace string
	constraints      []AccessConstraint
	permConstraints  map[string][]AccessConstraint
}

// NewResourceAccessChecker creates a checker that verifies permissions
// on individual resource instances in the given objectNamespace.
func NewResourceAccessChecker(
	auth security.Authorizer,
	objectNamespace string,
	opts ...ResourceCheckerOption,
) *ResourceAccessChecker {
	c := &ResourceAccessChecker{
		authorizer:       auth,
		objectNamespace:  objectNamespace,
		subjectNamespace: security.NamespaceProfile,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Check verifies that the caller (extracted from context claims) has the given
// permission on the resource identified by resourceID.
func (c *ResourceAccessChecker) Check(ctx context.Context, resourceID, permission string) error {
	claims := security.ClaimsFromContext(ctx)
	if claims == nil {
		return ErrInvalidSubject
	}

	subjectID, err := claims.GetSubject()
	if err != nil || subjectID == "" {
		return ErrInvalidSubject
	}

	return c.CheckSubject(ctx, resourceID, permission, subjectID)
}

// CheckSubject verifies that a specific subject has the given permission on
// the resource. Use this when the subject is not the authenticated caller.
func (c *ResourceAccessChecker) CheckSubject(ctx context.Context, resourceID, permission, subjectID string) error {
	req := security.CheckRequest{
		Object:     security.ObjectRef{Namespace: c.objectNamespace, ID: resourceID},
		Permission: permission,
		Subject:    security.SubjectRef{Namespace: c.subjectNamespace, ID: subjectID},
	}

	result, err := c.authorizer.Check(ctx, req)
	if err != nil {
		return err
	}

	if !result.Allowed {
		return NewPermissionDeniedError(req.Object, permission, req.Subject, result.Reason)
	}

	if cErr := evaluateConstraintsForPermission(
		ctx, c.constraints, c.permConstraints, permission, req.Object, req.Subject,
	); cErr != nil {
		util.Log(ctx).WithFields(map[string]any{
			"object_namespace": req.Object.Namespace,
			"object_id":        req.Object.ID,
			"permission":       permission,
			"subject_id":       subjectID,
			"denial_source":    "constraint",
		}).Info("authorization decision: denied by constraint")
		return cErr
	}

	return nil
}

// Grant assigns a relation to a subject on a resource instance.
func (c *ResourceAccessChecker) Grant(ctx context.Context, resourceID, relation, subjectID string) error {
	return c.authorizer.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: c.objectNamespace, ID: resourceID},
		Relation: relation,
		Subject:  security.SubjectRef{Namespace: c.subjectNamespace, ID: subjectID},
	})
}

// Revoke removes a relation from a subject on a resource instance.
func (c *ResourceAccessChecker) Revoke(ctx context.Context, resourceID, relation, subjectID string) error {
	return c.authorizer.DeleteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: c.objectNamespace, ID: resourceID},
		Relation: relation,
		Subject:  security.SubjectRef{Namespace: c.subjectNamespace, ID: subjectID},
	})
}

// Members returns all subjects with the given relation on a resource instance.
func (c *ResourceAccessChecker) Members(
	ctx context.Context,
	resourceID, relation string,
) ([]security.SubjectRef, error) {
	return c.authorizer.Expand(ctx, security.ObjectRef{
		Namespace: c.objectNamespace,
		ID:        resourceID,
	}, relation)
}
