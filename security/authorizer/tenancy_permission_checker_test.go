package authorizer_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pitabwire/frame/v3/security"
	"github.com/pitabwire/frame/v3/security/authorizer"
)

// claimsCtx simulates a request authenticated with a verified JWT issued
// for tenantID/partitionID.
func claimsCtx(subject, tenantID, partitionID string, roles ...string) context.Context {
	claims := &security.AuthenticationClaims{
		TenantID:    tenantID,
		PartitionID: partitionID,
		Roles:       roles,
	}
	claims.Subject = subject
	ctx := claims.ClaimsToContext(context.Background())
	return security.VerifiedTokenClaimsToContext(ctx, claims)
}

func tenancyPath(tenantID, partitionID string) string {
	return fmt.Sprintf("%s/%s", tenantID, partitionID)
}

// ---------------------------------------------------------------------------
// TenancyAccessChecker tests (using real Keto)
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestPermissionChecker_NoClaims() {
	adapter := s.newAdapter(nil)
	c := authorizer.NewTenancyAccessChecker(adapter, "resource")

	err := c.Check(context.Background(), "view")
	s.ErrorIs(err, authorizer.ErrInvalidSubject)
}

func (s *AuthorizerTestSuite) TestPermissionChecker_NoSubject() {
	adapter := s.newAdapter(nil)
	c := authorizer.NewTenancyAccessChecker(adapter, "resource")

	claims := &security.AuthenticationClaims{TenantID: "t1", PartitionID: "p1"}
	ctx := claims.ClaimsToContext(context.Background())
	err := c.Check(ctx, "view")
	s.ErrorIs(err, authorizer.ErrInvalidSubject)
}

func (s *AuthorizerTestSuite) TestPermissionChecker_NoTenant() {
	adapter := s.newAdapter(nil)
	c := authorizer.NewTenancyAccessChecker(adapter, "resource")

	claims := &security.AuthenticationClaims{PartitionID: "p1"}
	claims.Subject = "u1"
	ctx := claims.ClaimsToContext(context.Background())
	err := c.Check(ctx, "view")
	s.ErrorIs(err, authorizer.ErrInvalidObject)
}

func (s *AuthorizerTestSuite) TestPermissionChecker_NoPartition() {
	adapter := s.newAdapter(nil)
	c := authorizer.NewTenancyAccessChecker(adapter, "resource")

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
		Object:   security.ObjectRef{Namespace: "resource", ID: tp},
		Relation: "view",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: "pc-u1"},
	})
	s.Require().NoError(err)

	c := authorizer.NewTenancyAccessChecker(adapter, "resource")
	err = c.Check(claimsCtx("pc-u1", "pc-t1", "pc-p1", "user"), "view")
	s.NoError(err)
}

func (s *AuthorizerTestSuite) TestPermissionChecker_Denied() {
	adapter := s.newAdapter(nil)
	// Override default callback to return error so retry doesn't mask the denial.
	c := authorizer.NewTenancyAccessChecker(adapter, "resource",
		authorizer.WithOnTenancyAccessDenied(func(_ context.Context, _ security.Authorizer, _, _ string) error {
			return errors.New("no provisioning")
		}),
	)

	err := c.Check(claimsCtx("pc-u-denied", "pc-t-denied", "pc-p-denied", "user"), "view")
	s.Require().Error(err)

	var permErr *authorizer.PermissionDeniedError
	s.ErrorAs(err, &permErr)
}

func (s *AuthorizerTestSuite) TestPermissionChecker_SelfHeals() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	c := authorizer.NewTenancyAccessChecker(
		adapter,
		"resource",
		authorizer.WithSubjectNamespace("profile"),
		authorizer.WithOnTenancyAccessDenied(
			func(ctx context.Context, auth security.Authorizer, tenancyPath, subjectID string) error {
				return auth.WriteTuple(ctx, security.RelationTuple{
					Object:   security.ObjectRef{Namespace: "resource", ID: tenancyPath},
					Relation: "view",
					Subject:  security.SubjectRef{Namespace: "profile", ID: subjectID},
				})
			},
		),
	)

	err := c.Check(claimsCtx("pc-svc1", "pc-t-heal", "pc-p-heal", "system_internal"), "view")
	s.Require().NoError(err)

	// Verify tuple was provisioned.
	tp := tenancyPath("pc-t-heal", "pc-p-heal")
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "resource", ID: tp},
		Permission: "view",
		Subject:    security.SubjectRef{Namespace: "profile", ID: "pc-svc1"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed)
}

func (s *AuthorizerTestSuite) TestPermissionChecker_ProvisionFails() {
	adapter := s.newAdapter(nil)

	c := authorizer.NewTenancyAccessChecker(adapter, "resource",
		authorizer.WithSubjectNamespace("profile"),
		authorizer.WithOnTenancyAccessDenied(func(_ context.Context, _ security.Authorizer, _, _ string) error {
			return errors.New("provision failed")
		}),
	)

	err := c.Check(claimsCtx("pc-svc-pf", "pc-t-pf", "pc-p-pf", "system_internal"), "view")
	s.Require().Error(err)

	var permErr *authorizer.PermissionDeniedError
	s.ErrorAs(err, &permErr)
}

func (s *AuthorizerTestSuite) TestPermissionChecker_RetryDenied() {
	adapter := s.newAdapter(nil)

	// Provision succeeds but writes a different relation, not the one checked.
	c := authorizer.NewTenancyAccessChecker(
		adapter,
		"resource",
		authorizer.WithSubjectNamespace("profile"),
		authorizer.WithOnTenancyAccessDenied(
			func(ctx context.Context, auth security.Authorizer, tenancyPath, subjectID string) error {
				return auth.WriteTuple(ctx, security.RelationTuple{
					Object:   security.ObjectRef{Namespace: "resource", ID: tenancyPath},
					Relation: "other",
					Subject:  security.SubjectRef{Namespace: "profile", ID: subjectID},
				})
			},
		),
	)

	err := c.Check(claimsCtx("pc-svc-rd", "pc-t-rd", "pc-p-rd", "system_internal"), "view")
	s.Require().Error(err)

	var permErr *authorizer.PermissionDeniedError
	s.Require().ErrorAs(err, &permErr)
}

func (s *AuthorizerTestSuite) TestPermissionChecker_WithSubjectNamespace() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("pc-t-ns", "pc-p-ns")
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "resource", ID: tp},
		Relation: "view",
		Subject:  security.SubjectRef{Namespace: "custom", ID: "pc-u-ns"},
	})
	s.Require().NoError(err)

	c := authorizer.NewTenancyAccessChecker(adapter, "resource",
		authorizer.WithSubjectNamespace("custom"),
	)
	err = c.Check(claimsCtx("pc-u-ns", "pc-t-ns", "pc-p-ns", "user"), "view")
	s.Require().NoError(err)
}

func (s *AuthorizerTestSuite) TestPermissionChecker_DefaultCallbackRetries() {
	adapter := s.newAdapter(nil)
	// Non-internal denial: default callback logs only; retry still denied.
	c := authorizer.NewTenancyAccessChecker(adapter, "resource")

	err := c.Check(claimsCtx("pc-def", "pc-t-def", "pc-p-def", "user"), "view")
	s.Require().Error(err)

	var permErr *authorizer.PermissionDeniedError
	s.ErrorAs(err, &permErr)
}

func (s *AuthorizerTestSuite) TestPermissionChecker_DefaultSelfHealsInternalService() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	// Default callback writes tenancy_access#service for roles=["internal"].
	c := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access")

	err := c.CheckAccess(claimsCtx("pc-bot-heal", "pc-t-bot", "pc-p-bot", security.ConstantSystemInternalRole))
	s.Require().NoError(err)

	tp := tenancyPath("pc-t-bot", "pc-p-bot")
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: tp},
		Permission: "service",
		Subject:    security.SubjectRef{Namespace: security.NamespaceProfile, ID: "pc-bot-heal"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed)
}

// headerEnrichedCtx simulates an internal caller whose token is for its own
// partition but who names a different partition in X-Tenant-Id /
// X-Partition-Id headers, exactly as the auth interceptors do.
func headerEnrichedCtx(subject, hdrTenant, hdrPartition string) context.Context {
	ctx := claimsCtx(subject, "sh-own-t", "sh-own-p", security.ConstantSystemInternalRole)
	return security.EnrichTenancyClaims(ctx, hdrTenant, hdrPartition, "")
}

func (s *AuthorizerTestSuite) serviceTupleExists(path, subject string) bool {
	result, err := s.newAdapter(nil).Check(s.T().Context(), security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: path},
		Permission: "service",
		Subject:    security.SubjectRef{Namespace: security.NamespaceProfile, ID: subject},
	})
	s.Require().NoError(err)
	return result.Allowed
}

func (s *AuthorizerTestSuite) TestSelfHeal_RefusesHeaderNamedPartitionUnknownToKeto() {
	adapter := s.newAdapter(nil)
	c := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access")

	ctx := headerEnrichedCtx("sh-bot-ghost", "sh-ghost-t", "sh-ghost-p")
	err := c.CheckAccess(ctx)

	var permErr *authorizer.PermissionDeniedError
	s.Require().ErrorAs(err, &permErr, "a partition nobody registered must not be self-healed into existence")
	s.False(s.serviceTupleExists(tenancyPath("sh-ghost-t", "sh-ghost-p"), "sh-bot-ghost"),
		"no tenancy_access tuple may be written for an unverified partition")
}

func (s *AuthorizerTestSuite) TestSelfHeal_RefusesUnknownPartitionForCustomCallbacks() {
	adapter := s.newAdapter(nil)
	called := false
	c := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access",
		authorizer.WithOnTenancyAccessDenied(
			func(context.Context, security.Authorizer, string, string) error {
				called = true
				return nil
			}),
	)

	err := c.CheckAccess(headerEnrichedCtx("sh-bot-cb", "sh-cb-t", "sh-cb-p"))

	s.Require().Error(err)
	s.False(called, "provisioning callbacks must not run for unverified partitions")
}

func (s *AuthorizerTestSuite) TestSelfHeal_HealsHeaderNamedPartitionKnownToKeto() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)
	path := tenancyPath("sh-known-t", "sh-known-p")

	// The tenancy service registers partitions in Keto (members, parent
	// links, service grants); any tuple on the object proves it exists.
	s.Require().NoError(adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: path},
		Relation: "member",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: "sh-some-user"},
	}))

	c := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access")
	err := c.CheckAccess(headerEnrichedCtx("sh-bot-known", "sh-known-t", "sh-known-p"))

	s.Require().NoError(err)
	s.True(s.serviceTupleExists(path, "sh-bot-known"))
}

func (s *AuthorizerTestSuite) TestSelfHeal_HealsPartitionsCoveredByTokenClaims() {
	adapter := s.newAdapter(nil)
	c := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access")

	cases := []struct {
		name      string
		ctx       context.Context
		subject   string
		tenant    string
		partition string
	}{
		{
			name:      "token primary partition",
			ctx:       claimsCtx("sh-bot-tok", "sh-tok-t", "sh-tok-p", security.ConstantSystemInternalRole),
			subject:   "sh-bot-tok",
			tenant:    "sh-tok-t",
			partition: "sh-tok-p",
		},
		{
			name: "header names a partition listed in the token partition_ids",
			ctx: func() context.Context {
				claims := &security.AuthenticationClaims{
					TenantID:    "sh-multi-t",
					PartitionID: "sh-multi-p1",
					Roles:       []string{security.ConstantSystemInternalRole},
					Ext:         map[string]any{"partition_ids": []any{"sh-multi-p2"}},
				}
				claims.Subject = "sh-bot-multi"
				ctx := claims.ClaimsToContext(context.Background())
				ctx = security.VerifiedTokenClaimsToContext(ctx, claims)
				return security.EnrichTenancyClaims(ctx, "sh-multi-t", "sh-multi-p2", "")
			}(),
			subject:   "sh-bot-multi",
			tenant:    "sh-multi-t",
			partition: "sh-multi-p2",
		},
	}

	for _, tc := range cases {
		s.Run(tc.name, func() {
			s.Require().NoError(c.CheckAccess(tc.ctx))
			s.True(s.serviceTupleExists(tenancyPath(tc.tenant, tc.partition), tc.subject))
		})
	}
}

func (s *AuthorizerTestSuite) TestSelfHeal_CustomPartitionVerifier() {
	adapter := s.newAdapter(nil)
	verifierErr := errors.New("tenancy service unavailable")

	cases := []struct {
		name     string
		verifier authorizer.PartitionVerifierFunc
		wantHeal bool
	}{
		{name: "verifier confirms", wantHeal: true,
			verifier: func(context.Context, string, string) (bool, error) { return true, nil }},
		{name: "verifier rejects", wantHeal: false,
			verifier: func(context.Context, string, string) (bool, error) { return false, nil }},
		{name: "verifier fails closed", wantHeal: false,
			verifier: func(context.Context, string, string) (bool, error) { return false, verifierErr }},
	}

	for i, tc := range cases {
		s.Run(tc.name, func() {
			subject := fmt.Sprintf("sh-bot-ver-%d", i)
			partition := fmt.Sprintf("sh-ver-p-%d", i)
			c := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access",
				authorizer.WithPartitionVerifier(tc.verifier))

			err := c.CheckAccess(headerEnrichedCtx(subject, "sh-ver-t", partition))

			if tc.wantHeal {
				s.Require().NoError(err)
			} else {
				s.Require().Error(err)
			}
			s.Equal(tc.wantHeal, s.serviceTupleExists(tenancyPath("sh-ver-t", partition), subject))
		})
	}
}

// Queue consumers rebuild claims from message metadata, which carries the
// publisher's header-enriched tenancy. Those claims are not a verified token
// and must not count as covering the partition.
func (s *AuthorizerTestSuite) TestSelfHeal_QueueMetadataClaimsAreNotTrustedAsToken() {
	adapter := s.newAdapter(nil)
	c := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access")

	claims := security.ClaimsFromMap(map[string]string{
		"sub":          "sh-bot-queue",
		"profile_id":   "sh-bot-queue",
		"tenant_id":    "sh-queue-ghost-t",
		"partition_id": "sh-queue-ghost-p",
		"roles":        security.ConstantSystemInternalRole,
	})
	s.Require().NotNil(claims)
	s.Require().True(claims.IsInternalSystem(), "precondition: metadata claims carry the internal role")
	ctx := claims.ClaimsToContext(context.Background())

	err := c.CheckAccess(ctx)

	s.Require().Error(err)
	s.False(s.serviceTupleExists(tenancyPath("sh-queue-ghost-t", "sh-queue-ghost-p"), "sh-bot-queue"),
		"metadata-derived tenancy must not be self-healed without an existence check")
}

// Tuples written by self-heal (#service) must not count as evidence that a
// partition exists, otherwise one heal bootstraps the next.
func (s *AuthorizerTestSuite) TestSelfHeal_ServiceTuplesAreNotExistenceEvidence() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)
	path := tenancyPath("sh-svc-only-t", "sh-svc-only-p")
	s.Require().NoError(adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: path},
		Relation: "service",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: "sh-other-bot"},
	}))

	c := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access")
	err := c.CheckAccess(headerEnrichedCtx("sh-bot-svc-only", "sh-svc-only-t", "sh-svc-only-p"))

	s.Require().Error(err)
	s.False(s.serviceTupleExists(path, "sh-bot-svc-only"))
}

// A child partition linked to its parent (child#member <- parent#member) is
// registered even before anyone is granted on it directly.
func (s *AuthorizerTestSuite) TestSelfHeal_ParentLinkIsExistenceEvidence() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)
	child := tenancyPath("sh-link-t", "sh-link-child")
	s.Require().NoError(adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: child},
		Relation: "member",
		Subject: security.SubjectRef{
			Namespace: "tenancy_access", ID: tenancyPath("sh-link-t", "sh-link-parent"), Relation: "member",
		},
	}))

	c := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access")
	s.Require().NoError(c.CheckAccess(headerEnrichedCtx("sh-bot-link", "sh-link-t", "sh-link-child")))
	s.True(s.serviceTupleExists(child, "sh-bot-link"))
}

// A partition registered shortly after the first request (authz partition
// sync is asynchronous) is healed when a bounded retry is configured.
func (s *AuthorizerTestSuite) TestSelfHeal_RetryWaitsForAsyncPartitionRegistration() {
	adapter := s.newAdapter(nil)
	var calls int
	verifier := func(context.Context, string, string) (bool, error) {
		calls++
		return calls >= 3, nil
	}

	c := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access",
		authorizer.WithPartitionVerifier(verifier),
		authorizer.WithPartitionVerifyRetry(3, time.Millisecond))
	s.Require().NoError(c.CheckAccess(headerEnrichedCtx("sh-bot-retry", "sh-retry-t", "sh-retry-p")))
	s.Equal(3, calls)

	calls = -100
	c = authorizer.NewTenancyAccessChecker(adapter, "tenancy_access",
		authorizer.WithPartitionVerifier(verifier),
		authorizer.WithPartitionVerifyRetry(2, time.Millisecond))
	s.Require().Error(c.CheckAccess(headerEnrichedCtx("sh-bot-retry2", "sh-retry-t", "sh-retry-p2")))
	s.Equal(-98, calls, "attempts are bounded")

	reqCtx, cancel := context.WithCancel(headerEnrichedCtx("sh-bot-retry3", "sh-retry-t", "sh-retry-p3"))
	defer cancel()
	cancelCalls := 0
	cancellingVerifier := func(context.Context, string, string) (bool, error) {
		cancelCalls++
		cancel() // the request is abandoned while waiting for registration
		return false, nil
	}
	c = authorizer.NewTenancyAccessChecker(adapter, "tenancy_access",
		authorizer.WithPartitionVerifier(cancellingVerifier),
		authorizer.WithPartitionVerifyRetry(5, time.Hour))
	start := time.Now()
	s.Require().Error(c.CheckAccess(reqCtx))
	s.Equal(1, cancelCalls, "a cancelled context stops retrying")
	s.Less(time.Since(start), time.Minute, "the backoff wait is interrupted by cancellation")
}
