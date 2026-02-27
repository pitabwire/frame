package authorizer_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/security/authorizer"
)

func claimsCtx(subject, tenantID, partitionID string, roles ...string) context.Context {
	claims := &security.AuthenticationClaims{
		TenantID:    tenantID,
		PartitionID: partitionID,
		Roles:       roles,
	}
	claims.Subject = subject
	return claims.ClaimsToContext(context.Background())
}

func tenancyPath(tenantID, partitionID string) string {
	return fmt.Sprintf("%s/%s", tenantID, partitionID)
}

// ---------------------------------------------------------------------------
// TenancyAccessChecker tests (using real Keto)
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestPermissionChecker_NoClaims() {
	adapter := s.newAdapter(nil)
	c := authorizer.NewTenancyAccessChecker(adapter, "default")

	err := c.Check(context.Background(), "view")
	s.ErrorIs(err, authorizer.ErrInvalidSubject)
}

func (s *AuthorizerTestSuite) TestPermissionChecker_NoSubject() {
	adapter := s.newAdapter(nil)
	c := authorizer.NewTenancyAccessChecker(adapter, "default")

	claims := &security.AuthenticationClaims{TenantID: "t1", PartitionID: "p1"}
	ctx := claims.ClaimsToContext(context.Background())
	err := c.Check(ctx, "view")
	s.ErrorIs(err, authorizer.ErrInvalidSubject)
}

func (s *AuthorizerTestSuite) TestPermissionChecker_NoTenant() {
	adapter := s.newAdapter(nil)
	c := authorizer.NewTenancyAccessChecker(adapter, "default")

	claims := &security.AuthenticationClaims{PartitionID: "p1"}
	claims.Subject = "u1"
	ctx := claims.ClaimsToContext(context.Background())
	err := c.Check(ctx, "view")
	s.ErrorIs(err, authorizer.ErrInvalidObject)
}

func (s *AuthorizerTestSuite) TestPermissionChecker_NoPartition() {
	adapter := s.newAdapter(nil)
	c := authorizer.NewTenancyAccessChecker(adapter, "default")

	claims := &security.AuthenticationClaims{TenantID: "t1"}
	claims.Subject = "u1"
	ctx := claims.ClaimsToContext(context.Background())
	err := c.Check(ctx, "view")
	s.ErrorIs(err, authorizer.ErrInvalidObject)
}

func (s *AuthorizerTestSuite) TestPermissionChecker_Allowed() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("pc-t1", "pc-p1")
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "default", ID: tp},
		Relation: "view",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: "pc-u1"},
	})
	s.Require().NoError(err)

	c := authorizer.NewTenancyAccessChecker(adapter, "default")
	err = c.Check(claimsCtx("pc-u1", "pc-t1", "pc-p1", "user"), "view")
	s.NoError(err)
}

func (s *AuthorizerTestSuite) TestPermissionChecker_Denied() {
	adapter := s.newAdapter(nil)
	// Override default callback to return error so retry doesn't mask the denial.
	c := authorizer.NewTenancyAccessChecker(adapter, "default",
		authorizer.WithOnTenancyAccessDenied(func(_ context.Context, _ security.Authorizer, _, _ string) error {
			return errors.New("no provisioning")
		}),
	)

	err := c.Check(claimsCtx("pc-u-denied", "pc-t-denied", "pc-p-denied", "user"), "view")
	s.Error(err)

	var permErr *authorizer.PermissionDeniedError
	s.True(errors.As(err, &permErr))
}

func (s *AuthorizerTestSuite) TestPermissionChecker_SelfHeals() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	c := authorizer.NewTenancyAccessChecker(adapter, "default",
		authorizer.WithSubjectNamespace("profile"),
		authorizer.WithOnTenancyAccessDenied(func(ctx context.Context, auth security.Authorizer, tenancyPath, subjectID string) error {
			return auth.WriteTuple(ctx, security.RelationTuple{
				Object:   security.ObjectRef{Namespace: "default", ID: tenancyPath},
				Relation: "view",
				Subject:  security.SubjectRef{Namespace: "profile", ID: subjectID},
			})
		}),
	)

	err := c.Check(claimsCtx("pc-svc1", "pc-t-heal", "pc-p-heal", "system_internal"), "view")
	s.NoError(err)

	// Verify tuple was provisioned.
	tp := tenancyPath("pc-t-heal", "pc-p-heal")
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "default", ID: tp},
		Permission: "view",
		Subject:    security.SubjectRef{Namespace: "profile", ID: "pc-svc1"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed)
}

func (s *AuthorizerTestSuite) TestPermissionChecker_ProvisionFails() {
	adapter := s.newAdapter(nil)

	c := authorizer.NewTenancyAccessChecker(adapter, "default",
		authorizer.WithSubjectNamespace("profile"),
		authorizer.WithOnTenancyAccessDenied(func(_ context.Context, _ security.Authorizer, _, _ string) error {
			return errors.New("provision failed")
		}),
	)

	err := c.Check(claimsCtx("pc-svc-pf", "pc-t-pf", "pc-p-pf", "system_internal"), "view")
	s.Error(err)

	var permErr *authorizer.PermissionDeniedError
	s.True(errors.As(err, &permErr))
}

func (s *AuthorizerTestSuite) TestPermissionChecker_RetryDenied() {
	adapter := s.newAdapter(nil)

	// Provision succeeds but writes a different relation, not the one checked.
	c := authorizer.NewTenancyAccessChecker(adapter, "default",
		authorizer.WithSubjectNamespace("profile"),
		authorizer.WithOnTenancyAccessDenied(func(ctx context.Context, auth security.Authorizer, tenancyPath, subjectID string) error {
			return auth.WriteTuple(ctx, security.RelationTuple{
				Object:   security.ObjectRef{Namespace: "default", ID: tenancyPath},
				Relation: "other",
				Subject:  security.SubjectRef{Namespace: "profile", ID: subjectID},
			})
		}),
	)

	err := c.Check(claimsCtx("pc-svc-rd", "pc-t-rd", "pc-p-rd", "system_internal"), "view")
	s.Error(err)

	var permErr *authorizer.PermissionDeniedError
	s.True(errors.As(err, &permErr))
}

func (s *AuthorizerTestSuite) TestPermissionChecker_WithSubjectNamespace() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("pc-t-ns", "pc-p-ns")
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "default", ID: tp},
		Relation: "view",
		Subject:  security.SubjectRef{Namespace: "custom", ID: "pc-u-ns"},
	})
	s.Require().NoError(err)

	c := authorizer.NewTenancyAccessChecker(adapter, "default",
		authorizer.WithSubjectNamespace("custom"),
	)
	err = c.Check(claimsCtx("pc-u-ns", "pc-t-ns", "pc-p-ns", "user"), "view")
	s.NoError(err)
}

func (s *AuthorizerTestSuite) TestPermissionChecker_DefaultCallbackRetries() {
	adapter := s.newAdapter(nil)
	// Default callback logs and returns nil, which triggers a retry that fails.
	c := authorizer.NewTenancyAccessChecker(adapter, "default")

	err := c.Check(claimsCtx("pc-def", "pc-t-def", "pc-p-def", "system_internal"), "view")
	s.Error(err)

	var permErr *authorizer.PermissionDeniedError
	s.True(errors.As(err, &permErr))
}
