package authorizer_test

import (
	"context"
	"fmt"
	"time"

	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/security/authorizer"
)

// ===========================================================================
// Plane 1 — Data Access (tenancy_access)
//
// Verifies that profiles can be granted member or service access to a
// tenant/partition and that access is properly isolated between partitions.
// ===========================================================================

func (s *AuthorizerTestSuite) TestPlane1_MemberAccess() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("p1-t1", "p1-p1")
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "user-p1-member"},
	})
	s.Require().NoError(err)

	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: tp},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "user-p1-member"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "member should have access")
}

func (s *AuthorizerTestSuite) TestPlane1_ServiceAccess() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("p1-t2", "p1-p2")
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
		Relation: "service",
		Subject:  security.SubjectRef{ID: "svc-p1-bot"},
	})
	s.Require().NoError(err)

	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: tp},
		Permission: "service",
		Subject:    security.SubjectRef{ID: "svc-p1-bot"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "service account should have access")

	// Service account should NOT have member access
	result, err = adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: tp},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "svc-p1-bot"},
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "service should not be a member")
}

func (s *AuthorizerTestSuite) TestPlane1_NonMemberDenied() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: tenancyPath("p1-t3", "p1-p3")},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "stranger-p1"},
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "non-member should be denied")
}

func (s *AuthorizerTestSuite) TestPlane1_PartitionIsolation() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tpA := tenancyPath("p1-iso-t", "partition-A")
	tpB := tenancyPath("p1-iso-t", "partition-B")

	// Grant member to partition A only
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tpA},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "user-p1-iso"},
	})
	s.Require().NoError(err)

	resultA, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: tpA},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "user-p1-iso"},
	})
	s.Require().NoError(err)
	s.True(resultA.Allowed, "should have access to partition A")

	resultB, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: tpB},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "user-p1-iso"},
	})
	s.Require().NoError(err)
	s.False(resultB.Allowed, "should NOT have access to partition B")
}

func (s *AuthorizerTestSuite) TestPlane1_MixedMemberAndService() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("p1-mix-t", "p1-mix-p")

	tuples := []security.RelationTuple{
		{
			Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
			Relation: "member",
			Subject:  security.SubjectRef{ID: "user-p1-mix"},
		},
		{
			Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
			Relation: "service",
			Subject:  security.SubjectRef{ID: "svc-p1-mix"},
		},
		{
			Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
			Relation: "member",
			Subject:  security.SubjectRef{ID: "user-p1-mix2"},
		},
	}
	err := adapter.WriteTuples(ctx, tuples)
	s.Require().NoError(err)

	// Verify all three
	checks := []security.CheckRequest{
		{
			Object:     security.ObjectRef{Namespace: "tenancy_access", ID: tp},
			Permission: "member",
			Subject:    security.SubjectRef{ID: "user-p1-mix"},
		},
		{
			Object:     security.ObjectRef{Namespace: "tenancy_access", ID: tp},
			Permission: "service",
			Subject:    security.SubjectRef{ID: "svc-p1-mix"},
		},
		{
			Object:     security.ObjectRef{Namespace: "tenancy_access", ID: tp},
			Permission: "member",
			Subject:    security.SubjectRef{ID: "user-p1-mix2"},
		},
	}
	results, err := adapter.BatchCheck(ctx, checks)
	s.Require().NoError(err)
	s.Require().Len(results, 3)
	for i, r := range results {
		s.True(r.Allowed, "check %d should be allowed", i)
	}

	// List all relations
	listed, err := adapter.ListRelations(ctx, security.ObjectRef{Namespace: "tenancy_access", ID: tp})
	s.Require().NoError(err)
	s.Len(listed, 3, "should have 3 tuples: 2 members + 1 service")
}

func (s *AuthorizerTestSuite) TestPlane1_TenancyAccessChecker_Member() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("p1-chk-t", "p1-chk-p")
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
		Relation: "member",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: "user-p1-chk"},
	})
	s.Require().NoError(err)

	checker := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access")
	claimsCtxVal := claimsCtx("user-p1-chk", "p1-chk-t", "p1-chk-p", "user")
	err = checker.CheckAccess(claimsCtxVal)
	s.Require().NoError(err, "member should pass CheckAccess")
}

func (s *AuthorizerTestSuite) TestPlane1_TenancyAccessChecker_Service() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("p1-svc-t", "p1-svc-p")
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
		Relation: "service",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: "svc-p1-chk"},
	})
	s.Require().NoError(err)

	checker := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access")
	// "internal" role triggers IsInternalSystem() → checks "service" relation
	claimsCtxVal := claimsCtx("svc-p1-chk", "p1-svc-t", "p1-svc-p", "internal")
	err = checker.CheckAccess(claimsCtxVal)
	s.Require().NoError(err, "service account should pass CheckAccess with service relation")
}

func (s *AuthorizerTestSuite) TestPlane1_TenancyAccessChecker_ServiceDeniedAsMember() {
	adapter := s.newAdapter(nil)

	// Service tuple exists but caller is NOT internal → checks "member" relation
	ctx := s.T().Context()
	tp := tenancyPath("p1-svcm-t", "p1-svcm-p")
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
		Relation: "service",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: "svc-p1-asmember"},
	})
	s.Require().NoError(err)

	checker := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access")
	claimsCtxVal := claimsCtx("svc-p1-asmember", "p1-svcm-t", "p1-svcm-p", "user")
	err = checker.CheckAccess(claimsCtxVal)
	s.Require().Error(err, "non-internal caller should fail member check even if service tuple exists")
}

func (s *AuthorizerTestSuite) TestPlane1_TenancyAccessChecker_Denied() {
	adapter := s.newAdapter(nil)

	checker := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access")
	claimsCtxVal := claimsCtx("nobody-p1", "p1-deny-t", "p1-deny-p", "user")
	err := checker.CheckAccess(claimsCtxVal)
	s.Require().Error(err, "should be denied when no tuple exists")

	var permErr *authorizer.PermissionDeniedError
	s.Require().ErrorAs(err, &permErr)
}

func (s *AuthorizerTestSuite) TestPlane1_MultiplePartitionsPerTenant() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tenant := "p1-multi-t"
	partitions := []string{"partition-1", "partition-2", "partition-3"}

	// Grant access to all partitions
	for _, p := range partitions {
		tp := tenancyPath(tenant, p)
		err := adapter.WriteTuple(ctx, security.RelationTuple{
			Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
			Relation: "member",
			Subject:  security.SubjectRef{ID: "user-p1-multi"},
		})
		s.Require().NoError(err)
	}

	// Verify access to all partitions
	for _, p := range partitions {
		tp := tenancyPath(tenant, p)
		result, err := adapter.Check(ctx, security.CheckRequest{
			Object:     security.ObjectRef{Namespace: "tenancy_access", ID: tp},
			Permission: "member",
			Subject:    security.SubjectRef{ID: "user-p1-multi"},
		})
		s.Require().NoError(err)
		s.True(result.Allowed, "should have access to %s", p)
	}

	// Denied for unknown partition
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: tenancyPath(tenant, "nonexistent")},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "user-p1-multi"},
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "should be denied for unknown partition")
}

func (s *AuthorizerTestSuite) TestPlane1_RevokeAccess() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("p1-rev-t", "p1-rev-p")
	tuple := security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "user-p1-rev"},
	}

	err := adapter.WriteTuple(ctx, tuple)
	s.Require().NoError(err)

	result, err := adapter.Check(ctx, security.CheckRequest{
		Object: tuple.Object, Permission: "member", Subject: tuple.Subject,
	})
	s.Require().NoError(err)
	s.True(result.Allowed)

	err = adapter.DeleteTuple(ctx, tuple)
	s.Require().NoError(err)

	result, err = adapter.Check(ctx, security.CheckRequest{
		Object: tuple.Object, Permission: "member", Subject: tuple.Subject,
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "should be denied after revocation")
}

// ---------------------------------------------------------------------------
// Plane 1 — Partition Inheritance
//
// Members of a parent partition inherit access to child partitions via
// SubjectSet chains. The reverse does NOT hold — child members cannot
// access the parent.
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestPlane1_Inheritance_ParentMemberAccessesChild() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tenant := "p1-inh-t"
	parentTP := tenancyPath(tenant, "parent")
	childTP := tenancyPath(tenant, "child")

	// Link child → parent: child#member@(parent#member)
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: childTP},
		Relation: "member",
		Subject:  security.SubjectRef{Namespace: "tenancy_access", ID: parentTP, Relation: "member"},
	})
	s.Require().NoError(err)

	// Grant user membership on the parent
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: parentTP},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "user-p1-inh"},
	})
	s.Require().NoError(err)

	// Parent member can access child
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: childTP},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "user-p1-inh"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "parent member should access child partition")

	// Parent member can still access parent
	result, err = adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: parentTP},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "user-p1-inh"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "parent member should access parent partition")
}

func (s *AuthorizerTestSuite) TestPlane1_Inheritance_ChildMemberCannotAccessParent() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tenant := "p1-inh-rev-t"
	parentTP := tenancyPath(tenant, "parent")
	childTP := tenancyPath(tenant, "child")

	// Link child → parent
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: childTP},
		Relation: "member",
		Subject:  security.SubjectRef{Namespace: "tenancy_access", ID: parentTP, Relation: "member"},
	})
	s.Require().NoError(err)

	// Grant user membership on the CHILD only (not the parent)
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: childTP},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "user-p1-child-only"},
	})
	s.Require().NoError(err)

	// Child member can access child
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: childTP},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "user-p1-child-only"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "child member should access child partition")

	// Child member CANNOT access parent
	result, err = adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: parentTP},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "user-p1-child-only"},
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "child member must NOT access parent partition")
}

func (s *AuthorizerTestSuite) TestPlane1_Inheritance_ServiceAccountInherits() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tenant := "p1-inh-svc-t"
	parentTP := tenancyPath(tenant, "parent")
	childTP := tenancyPath(tenant, "child")

	// Link child → parent for service relation
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: childTP},
		Relation: "service",
		Subject:  security.SubjectRef{Namespace: "tenancy_access", ID: parentTP, Relation: "service"},
	})
	s.Require().NoError(err)

	// Grant service on parent
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: parentTP},
		Relation: "service",
		Subject:  security.SubjectRef{ID: "svc-p1-inh"},
	})
	s.Require().NoError(err)

	// Service inherits into child
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: childTP},
		Permission: "service",
		Subject:    security.SubjectRef{ID: "svc-p1-inh"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "service on parent should inherit to child")

	// Service on child does NOT access parent
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: childTP},
		Relation: "service",
		Subject:  security.SubjectRef{ID: "svc-p1-child-only"},
	})
	s.Require().NoError(err)

	result, err = adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: parentTP},
		Permission: "service",
		Subject:    security.SubjectRef{ID: "svc-p1-child-only"},
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "child-only service must NOT access parent")
}

func (s *AuthorizerTestSuite) TestPlane1_Inheritance_DeepHierarchy() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tenant := "p1-deep-t"
	rootTP := tenancyPath(tenant, "root")
	midTP := tenancyPath(tenant, "mid")
	leafTP := tenancyPath(tenant, "leaf")

	// root → mid → leaf
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: midTP},
		Relation: "member",
		Subject:  security.SubjectRef{Namespace: "tenancy_access", ID: rootTP, Relation: "member"},
	})
	s.Require().NoError(err)

	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: leafTP},
		Relation: "member",
		Subject:  security.SubjectRef{Namespace: "tenancy_access", ID: midTP, Relation: "member"},
	})
	s.Require().NoError(err)

	// Grant member on root
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: rootTP},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "user-p1-deep"},
	})
	s.Require().NoError(err)

	// Root member reaches all levels
	for _, tp := range []string{rootTP, midTP, leafTP} {
		result, checkErr := adapter.Check(ctx, security.CheckRequest{
			Object:     security.ObjectRef{Namespace: "tenancy_access", ID: tp},
			Permission: "member",
			Subject:    security.SubjectRef{ID: "user-p1-deep"},
		})
		s.Require().NoError(checkErr)
		s.True(result.Allowed, "root member should reach %s", tp)
	}

	// Mid-level member reaches mid and leaf but not root
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: midTP},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "user-p1-mid"},
	})
	s.Require().NoError(err)

	midResult, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: midTP},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "user-p1-mid"},
	})
	s.Require().NoError(err)
	s.True(midResult.Allowed, "mid member should access mid")

	leafResult, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: leafTP},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "user-p1-mid"},
	})
	s.Require().NoError(err)
	s.True(leafResult.Allowed, "mid member should access leaf")

	rootResult, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: rootTP},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "user-p1-mid"},
	})
	s.Require().NoError(err)
	s.False(rootResult.Allowed, "mid member must NOT access root")
}

func (s *AuthorizerTestSuite) TestPlane1_Inheritance_SiblingIsolation() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tenant := "p1-sib-t"
	parentTP := tenancyPath(tenant, "parent")
	childATP := tenancyPath(tenant, "child-a")
	childBTP := tenancyPath(tenant, "child-b")

	// Both children inherit from parent
	for _, childTP := range []string{childATP, childBTP} {
		err := adapter.WriteTuple(ctx, security.RelationTuple{
			Object:   security.ObjectRef{Namespace: "tenancy_access", ID: childTP},
			Relation: "member",
			Subject:  security.SubjectRef{Namespace: "tenancy_access", ID: parentTP, Relation: "member"},
		})
		s.Require().NoError(err)
	}

	// User A is a direct member of child-a only
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: childATP},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "user-p1-sib-a"},
	})
	s.Require().NoError(err)

	// User A can access child-a
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: childATP},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "user-p1-sib-a"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed)

	// User A CANNOT access sibling child-b
	result, err = adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: childBTP},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "user-p1-sib-a"},
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "child-a member must NOT access sibling child-b")

	// Parent member reaches BOTH children
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: parentTP},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "user-p1-sib-parent"},
	})
	s.Require().NoError(err)

	for _, childTP := range []string{childATP, childBTP} {
		checkResult, checkErr := adapter.Check(ctx, security.CheckRequest{
			Object:     security.ObjectRef{Namespace: "tenancy_access", ID: childTP},
			Permission: "member",
			Subject:    security.SubjectRef{ID: "user-p1-sib-parent"},
		})
		s.Require().NoError(checkErr)
		s.True(checkResult.Allowed, "parent member should reach %s", childTP)
	}
}

func (s *AuthorizerTestSuite) TestPlane1_Inheritance_TenancyAccessChecker() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tenant := "p1-inh-chk-t"
	parentTP := tenancyPath(tenant, "parent")
	childTP := tenancyPath(tenant, "child")

	// Link child → parent
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: childTP},
		Relation: "member",
		Subject:  security.SubjectRef{Namespace: "tenancy_access", ID: parentTP, Relation: "member"},
	})
	s.Require().NoError(err)

	// Grant parent membership
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: parentTP},
		Relation: "member",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: "user-p1-inh-chk"},
	})
	s.Require().NoError(err)

	checker := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access")

	// Parent access works
	parentCtx := claimsCtx("user-p1-inh-chk", tenant, "parent", "user")
	err = checker.CheckAccess(parentCtx)
	s.Require().NoError(err, "should pass parent CheckAccess")

	// Child access works via inheritance
	childCtx := claimsCtx("user-p1-inh-chk", tenant, "child", "user")
	err = checker.CheckAccess(childCtx)
	s.Require().NoError(err, "should pass child CheckAccess via inheritance")
}

func (s *AuthorizerTestSuite) TestPlane1_Inheritance_RevokeParentRevokesChild() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tenant := "p1-inh-revp-t"
	parentTP := tenancyPath(tenant, "parent")
	childTP := tenancyPath(tenant, "child")

	// Link child → parent
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: childTP},
		Relation: "member",
		Subject:  security.SubjectRef{Namespace: "tenancy_access", ID: parentTP, Relation: "member"},
	})
	s.Require().NoError(err)

	// Grant parent membership
	parentTuple := security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: parentTP},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "user-p1-inh-revp"},
	}
	err = adapter.WriteTuple(ctx, parentTuple)
	s.Require().NoError(err)

	// Confirm child access via inheritance
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: childTP},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "user-p1-inh-revp"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "should have child access before revocation")

	// Revoke parent membership
	err = adapter.DeleteTuple(ctx, parentTuple)
	s.Require().NoError(err)

	// Child access is gone too
	result, err = adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenancy_access", ID: childTP},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "user-p1-inh-revp"},
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "revoking parent membership should revoke inherited child access")
}

// ===========================================================================
// Plane 2 — Functional Permissions (service_tenancy, service_profile)
//
// Verifies role-based permission resolution via OPL permits.
// Roles: owner > admin > member. Permissions are computed server-side.
// Service accounts use explicit granted_X tuples (least privilege).
// ===========================================================================

func (s *AuthorizerTestSuite) TestPlane2_OwnerHasAllPermissions() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("p2-own-t", "p2-own-p")
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tp},
		Relation: "owner",
		Subject:  security.SubjectRef{ID: "alice-p2-own"},
	})
	s.Require().NoError(err)

	allPerms := []string{"create", "read", "update", "delete", "list"}
	checks := make([]security.CheckRequest, len(allPerms))
	for i, p := range allPerms {
		checks[i] = security.CheckRequest{
			Object:     security.ObjectRef{Namespace: "service_tenancy", ID: tp},
			Permission: p,
			Subject:    security.SubjectRef{ID: "alice-p2-own"},
		}
	}

	results, err := adapter.BatchCheck(ctx, checks)
	s.Require().NoError(err)
	s.Require().Len(results, len(allPerms))
	for i, r := range results {
		s.True(r.Allowed, "owner should have %s permission", allPerms[i])
	}
}

func (s *AuthorizerTestSuite) TestPlane2_AdminPermissions() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("p2-adm-t", "p2-adm-p")
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tp},
		Relation: "admin",
		Subject:  security.SubjectRef{ID: "bob-p2-adm"},
	})
	s.Require().NoError(err)

	type permCheck struct {
		perm    string
		allowed bool
	}

	expected := []permCheck{
		{"create", true},  // admin can create
		{"read", true},    // admin can read
		{"update", true},  // admin can update
		{"delete", false}, // only owner can delete
		{"list", true},    // admin can list
	}

	for _, tc := range expected {
		result, checkErr := adapter.Check(ctx, security.CheckRequest{
			Object:     security.ObjectRef{Namespace: "service_tenancy", ID: tp},
			Permission: tc.perm,
			Subject:    security.SubjectRef{ID: "bob-p2-adm"},
		})
		s.Require().NoError(checkErr)
		s.Equal(tc.allowed, result.Allowed, "admin %s should be %v", tc.perm, tc.allowed)
	}
}

func (s *AuthorizerTestSuite) TestPlane2_MemberPermissions() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("p2-mem-t", "p2-mem-p")
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tp},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "charlie-p2-mem"},
	})
	s.Require().NoError(err)

	type permCheck struct {
		perm    string
		allowed bool
	}

	expected := []permCheck{
		{"create", false}, // member cannot create
		{"read", true},    // member can read
		{"update", false}, // member cannot update
		{"delete", false}, // member cannot delete
		{"list", true},    // member can list
	}

	for _, tc := range expected {
		result, checkErr := adapter.Check(ctx, security.CheckRequest{
			Object:     security.ObjectRef{Namespace: "service_tenancy", ID: tp},
			Permission: tc.perm,
			Subject:    security.SubjectRef{ID: "charlie-p2-mem"},
		})
		s.Require().NoError(checkErr)
		s.Equal(tc.allowed, result.Allowed, "member %s should be %v", tc.perm, tc.allowed)
	}
}

func (s *AuthorizerTestSuite) TestPlane2_ServiceAccountExplicitGrants() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("p2-svc-t", "p2-svc-p")

	// Service account gets only granted_read and granted_list (least privilege)
	for _, rel := range []string{"granted_read", "granted_list"} {
		err := adapter.WriteTuple(ctx, security.RelationTuple{
			Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tp},
			Relation: rel,
			Subject:  security.SubjectRef{ID: "svc-p2-bot"},
		})
		s.Require().NoError(err)
	}

	type permCheck struct {
		perm    string
		allowed bool
	}

	expected := []permCheck{
		{"create", false}, // not granted
		{"read", true},    // granted_read
		{"update", false}, // not granted
		{"delete", false}, // not granted
		{"list", true},    // granted_list
	}

	for _, tc := range expected {
		result, checkErr := adapter.Check(ctx, security.CheckRequest{
			Object:     security.ObjectRef{Namespace: "service_tenancy", ID: tp},
			Permission: tc.perm,
			Subject:    security.SubjectRef{ID: "svc-p2-bot"},
		})
		s.Require().NoError(checkErr)
		s.Equal(tc.allowed, result.Allowed, "svc %s should be %v", tc.perm, tc.allowed)
	}
}

func (s *AuthorizerTestSuite) TestPlane2_CrossServiceIsolation() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("p2-xsvc-t", "p2-xsvc-p")

	// Grant owner in service_tenancy
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tp},
		Relation: "owner",
		Subject:  security.SubjectRef{ID: "user-p2-xsvc"},
	})
	s.Require().NoError(err)

	// Owner can create in service_tenancy
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "service_tenancy", ID: tp},
		Permission: "create",
		Subject:    security.SubjectRef{ID: "user-p2-xsvc"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "should have create in service_tenancy")

	// Same user should NOT have any permissions in service_profile
	result, err = adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "service_profile", ID: tp},
		Permission: "create",
		Subject:    security.SubjectRef{ID: "user-p2-xsvc"},
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "service_tenancy owner should NOT have access in service_profile")
}

func (s *AuthorizerTestSuite) TestPlane2_ServiceProfilePermissions() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("p2-prof-t", "p2-prof-p")

	// Owner of service_profile
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "service_profile", ID: tp},
		Relation: "owner",
		Subject:  security.SubjectRef{ID: "alice-p2-prof"},
	})
	s.Require().NoError(err)

	// Admin of service_profile
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "service_profile", ID: tp},
		Relation: "admin",
		Subject:  security.SubjectRef{ID: "bob-p2-prof"},
	})
	s.Require().NoError(err)

	// service_profile has: create, read, update, delete
	type userPerm struct {
		user    string
		perm    string
		allowed bool
	}

	expected := []userPerm{
		// Owner has all
		{"alice-p2-prof", "create", true},
		{"alice-p2-prof", "read", true},
		{"alice-p2-prof", "update", true},
		{"alice-p2-prof", "delete", true},
		// Admin has create, read, update but not delete
		{"bob-p2-prof", "create", true},
		{"bob-p2-prof", "read", true},
		{"bob-p2-prof", "update", true},
		{"bob-p2-prof", "delete", false},
	}

	for _, tc := range expected {
		result, checkErr := adapter.Check(ctx, security.CheckRequest{
			Object:     security.ObjectRef{Namespace: "service_profile", ID: tp},
			Permission: tc.perm,
			Subject:    security.SubjectRef{ID: tc.user},
		})
		s.Require().NoError(checkErr)
		s.Equal(tc.allowed, result.Allowed, "%s %s should be %v", tc.user, tc.perm, tc.allowed)
	}
}

func (s *AuthorizerTestSuite) TestPlane2_FunctionChecker_Integration() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("p2-fc-t", "p2-fc-p")
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tp},
		Relation: "admin",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: "user-p2-fc"},
	})
	s.Require().NoError(err)

	checker := authorizer.NewFunctionChecker(adapter, "service_tenancy")
	claimsCtxVal := claimsCtx("user-p2-fc", "p2-fc-t", "p2-fc-p", "user")

	// Admin can create
	err = checker.Check(claimsCtxVal, "create")
	s.Require().NoError(err, "admin should have create permission")

	// Admin cannot delete
	err = checker.Check(claimsCtxVal, "delete")
	s.Require().Error(err, "admin should NOT have delete permission")

	var permErr *authorizer.PermissionDeniedError
	s.Require().ErrorAs(err, &permErr)
}

func (s *AuthorizerTestSuite) TestPlane2_FunctionChecker_MissingClaims() {
	adapter := s.newAdapter(nil)
	checker := authorizer.NewFunctionChecker(adapter, "service_tenancy")

	err := checker.Check(context.Background(), "read")
	s.Require().ErrorIs(err, authorizer.ErrInvalidSubject)
}

func (s *AuthorizerTestSuite) TestPlane2_FunctionChecker_MissingTenant() {
	adapter := s.newAdapter(nil)
	checker := authorizer.NewFunctionChecker(adapter, "service_tenancy")

	claims := &security.AuthenticationClaims{PartitionID: "p1"}
	claims.Subject = "u1"
	ctx := claims.ClaimsToContext(context.Background())

	err := checker.Check(ctx, "read")
	s.Require().ErrorIs(err, authorizer.ErrInvalidObject)
}

func (s *AuthorizerTestSuite) TestPlane2_MultipleRolesInSamePartition() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("p2-multi-t", "p2-multi-p")

	// Assign different roles to different users in the same partition
	tuples := []security.RelationTuple{
		{
			Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tp},
			Relation: "owner",
			Subject:  security.SubjectRef{ID: "owner-p2-multi"},
		},
		{
			Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tp},
			Relation: "admin",
			Subject:  security.SubjectRef{ID: "admin-p2-multi"},
		},
		{
			Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tp},
			Relation: "member",
			Subject:  security.SubjectRef{ID: "member-p2-multi"},
		},
	}
	err := adapter.WriteTuples(ctx, tuples)
	s.Require().NoError(err)

	// owner: delete=yes, admin: delete=no, member: delete=no
	for _, tc := range []struct {
		user    string
		allowed bool
	}{
		{"owner-p2-multi", true},
		{"admin-p2-multi", false},
		{"member-p2-multi", false},
	} {
		result, checkErr := adapter.Check(ctx, security.CheckRequest{
			Object:     security.ObjectRef{Namespace: "service_tenancy", ID: tp},
			Permission: "delete",
			Subject:    security.SubjectRef{ID: tc.user},
		})
		s.Require().NoError(checkErr)
		s.Equal(tc.allowed, result.Allowed, "%s delete should be %v", tc.user, tc.allowed)
	}
}

func (s *AuthorizerTestSuite) TestPlane2_PartitionIsolation() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tpA := tenancyPath("p2-iso-t", "partition-A")
	tpB := tenancyPath("p2-iso-t", "partition-B")

	// Owner in partition A
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tpA},
		Relation: "owner",
		Subject:  security.SubjectRef{ID: "user-p2-iso"},
	})
	s.Require().NoError(err)

	resultA, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "service_tenancy", ID: tpA},
		Permission: "delete",
		Subject:    security.SubjectRef{ID: "user-p2-iso"},
	})
	s.Require().NoError(err)
	s.True(resultA.Allowed, "owner should have delete in partition A")

	resultB, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "service_tenancy", ID: tpB},
		Permission: "delete",
		Subject:    security.SubjectRef{ID: "user-p2-iso"},
	})
	s.Require().NoError(err)
	s.False(resultB.Allowed, "should NOT have permissions in partition B")
}

func (s *AuthorizerTestSuite) TestPlane2_RevokeRole() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("p2-rev-t", "p2-rev-p")
	tuple := security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tp},
		Relation: "owner",
		Subject:  security.SubjectRef{ID: "user-p2-rev"},
	}

	err := adapter.WriteTuple(ctx, tuple)
	s.Require().NoError(err)

	result, err := adapter.Check(ctx, security.CheckRequest{
		Object: tuple.Object, Permission: "delete", Subject: tuple.Subject,
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "owner should have delete before revocation")

	err = adapter.DeleteTuple(ctx, tuple)
	s.Require().NoError(err)

	result, err = adapter.Check(ctx, security.CheckRequest{
		Object: tuple.Object, Permission: "delete", Subject: tuple.Subject,
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "should lose delete after role revocation")
}

// ===========================================================================
// Plane 3 — Resource Access (chat_room, file)
//
// Verifies per-resource-instance permissions via OPL permits.
// chat_room: owner > admin > member > viewer with computed permissions.
// file: owner > editor > viewer with computed permissions.
// ===========================================================================

func (s *AuthorizerTestSuite) TestPlane3_ChatRoom_OwnerPermissions() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "chat_room", ID: "room-p3-own"},
		Relation: "owner",
		Subject:  security.SubjectRef{ID: "alice-p3-room"},
	})
	s.Require().NoError(err)

	type permCheck struct {
		perm    string
		allowed bool
	}

	expected := []permCheck{
		{"send_message", true},
		{"read", true},
		{"manage", true},
		{"delete", true},
		{"invite", true},
	}

	for _, tc := range expected {
		result, checkErr := adapter.Check(ctx, security.CheckRequest{
			Object:     security.ObjectRef{Namespace: "chat_room", ID: "room-p3-own"},
			Permission: tc.perm,
			Subject:    security.SubjectRef{ID: "alice-p3-room"},
		})
		s.Require().NoError(checkErr)
		s.Equal(tc.allowed, result.Allowed, "room owner %s should be %v", tc.perm, tc.allowed)
	}
}

func (s *AuthorizerTestSuite) TestPlane3_ChatRoom_AdminPermissions() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "chat_room", ID: "room-p3-adm"},
		Relation: "admin",
		Subject:  security.SubjectRef{ID: "bob-p3-room"},
	})
	s.Require().NoError(err)

	type permCheck struct {
		perm    string
		allowed bool
	}

	expected := []permCheck{
		{"send_message", true},
		{"read", true},
		{"manage", true},
		{"delete", false}, // only owner
		{"invite", true},
	}

	for _, tc := range expected {
		result, checkErr := adapter.Check(ctx, security.CheckRequest{
			Object:     security.ObjectRef{Namespace: "chat_room", ID: "room-p3-adm"},
			Permission: tc.perm,
			Subject:    security.SubjectRef{ID: "bob-p3-room"},
		})
		s.Require().NoError(checkErr)
		s.Equal(tc.allowed, result.Allowed, "room admin %s should be %v", tc.perm, tc.allowed)
	}
}

func (s *AuthorizerTestSuite) TestPlane3_ChatRoom_MemberPermissions() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "chat_room", ID: "room-p3-mem"},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "charlie-p3-room"},
	})
	s.Require().NoError(err)

	type permCheck struct {
		perm    string
		allowed bool
	}

	expected := []permCheck{
		{"send_message", true},
		{"read", true},
		{"manage", false}, // admin+ only
		{"delete", false}, // owner only
		{"invite", false}, // admin+ only
	}

	for _, tc := range expected {
		result, checkErr := adapter.Check(ctx, security.CheckRequest{
			Object:     security.ObjectRef{Namespace: "chat_room", ID: "room-p3-mem"},
			Permission: tc.perm,
			Subject:    security.SubjectRef{ID: "charlie-p3-room"},
		})
		s.Require().NoError(checkErr)
		s.Equal(tc.allowed, result.Allowed, "room member %s should be %v", tc.perm, tc.allowed)
	}
}

func (s *AuthorizerTestSuite) TestPlane3_ChatRoom_ViewerPermissions() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "chat_room", ID: "room-p3-view"},
		Relation: "viewer",
		Subject:  security.SubjectRef{ID: "dave-p3-room"},
	})
	s.Require().NoError(err)

	type permCheck struct {
		perm    string
		allowed bool
	}

	expected := []permCheck{
		{"send_message", false}, // member+ only
		{"read", true},          // viewer can read
		{"manage", false},
		{"delete", false},
		{"invite", false},
	}

	for _, tc := range expected {
		result, checkErr := adapter.Check(ctx, security.CheckRequest{
			Object:     security.ObjectRef{Namespace: "chat_room", ID: "room-p3-view"},
			Permission: tc.perm,
			Subject:    security.SubjectRef{ID: "dave-p3-room"},
		})
		s.Require().NoError(checkErr)
		s.Equal(tc.allowed, result.Allowed, "room viewer %s should be %v", tc.perm, tc.allowed)
	}
}

func (s *AuthorizerTestSuite) TestPlane3_ChatRoom_NonMemberDenied() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	perms := []string{"send_message", "read", "manage", "delete", "invite"}
	for _, p := range perms {
		result, err := adapter.Check(ctx, security.CheckRequest{
			Object:     security.ObjectRef{Namespace: "chat_room", ID: "room-p3-deny"},
			Permission: p,
			Subject:    security.SubjectRef{ID: "stranger-p3"},
		})
		s.Require().NoError(err)
		s.False(result.Allowed, "non-member should be denied %s", p)
	}
}

func (s *AuthorizerTestSuite) TestPlane3_ChatRoom_InstanceIsolation() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	// Alice is member of room A only
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "chat_room", ID: "room-iso-A"},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "alice-p3-iso"},
	})
	s.Require().NoError(err)

	resultA, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "chat_room", ID: "room-iso-A"},
		Permission: "send_message",
		Subject:    security.SubjectRef{ID: "alice-p3-iso"},
	})
	s.Require().NoError(err)
	s.True(resultA.Allowed, "should have access in room A")

	resultB, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "chat_room", ID: "room-iso-B"},
		Permission: "send_message",
		Subject:    security.SubjectRef{ID: "alice-p3-iso"},
	})
	s.Require().NoError(err)
	s.False(resultB.Allowed, "should NOT have access in room B")
}

func (s *AuthorizerTestSuite) TestPlane3_ChatRoom_GrantRevokeLifecycle() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	room := security.ObjectRef{Namespace: "chat_room", ID: "room-p3-lifecycle"}

	// Step 1: Grant member
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object: room, Relation: "member", Subject: security.SubjectRef{ID: "user-p3-lc"},
	})
	s.Require().NoError(err)

	result, err := adapter.Check(ctx, security.CheckRequest{
		Object: room, Permission: "send_message", Subject: security.SubjectRef{ID: "user-p3-lc"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "should send messages as member")

	// Step 2: Promote to admin
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object: room, Relation: "admin", Subject: security.SubjectRef{ID: "user-p3-lc"},
	})
	s.Require().NoError(err)

	result, err = adapter.Check(ctx, security.CheckRequest{
		Object: room, Permission: "manage", Subject: security.SubjectRef{ID: "user-p3-lc"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "should manage as admin")

	// Step 3: Revoke admin, keep member
	err = adapter.DeleteTuple(ctx, security.RelationTuple{
		Object: room, Relation: "admin", Subject: security.SubjectRef{ID: "user-p3-lc"},
	})
	s.Require().NoError(err)

	result, err = adapter.Check(ctx, security.CheckRequest{
		Object: room, Permission: "manage", Subject: security.SubjectRef{ID: "user-p3-lc"},
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "should lose manage after admin revoked")

	result, err = adapter.Check(ctx, security.CheckRequest{
		Object: room, Permission: "send_message", Subject: security.SubjectRef{ID: "user-p3-lc"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "should still send messages as member")

	// Step 4: Revoke member too
	err = adapter.DeleteTuple(ctx, security.RelationTuple{
		Object: room, Relation: "member", Subject: security.SubjectRef{ID: "user-p3-lc"},
	})
	s.Require().NoError(err)

	result, err = adapter.Check(ctx, security.CheckRequest{
		Object: room, Permission: "send_message", Subject: security.SubjectRef{ID: "user-p3-lc"},
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "should lose all access after full revocation")
}

func (s *AuthorizerTestSuite) TestPlane3_ChatRoom_MultipleUsersPerRoom() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	room := security.ObjectRef{Namespace: "chat_room", ID: "room-p3-multi"}

	tuples := []security.RelationTuple{
		{Object: room, Relation: "owner", Subject: security.SubjectRef{ID: "owner-p3-multi"}},
		{Object: room, Relation: "admin", Subject: security.SubjectRef{ID: "admin-p3-multi"}},
		{Object: room, Relation: "member", Subject: security.SubjectRef{ID: "member-p3-multi"}},
		{Object: room, Relation: "viewer", Subject: security.SubjectRef{ID: "viewer-p3-multi"}},
	}
	err := adapter.WriteTuples(ctx, tuples)
	s.Require().NoError(err)

	type userPerm struct {
		user    string
		perm    string
		allowed bool
	}

	expected := []userPerm{
		// Owner: all
		{"owner-p3-multi", "delete", true},
		{"owner-p3-multi", "manage", true},
		{"owner-p3-multi", "send_message", true},
		{"owner-p3-multi", "read", true},
		// Admin: manage, send, read but not delete
		{"admin-p3-multi", "delete", false},
		{"admin-p3-multi", "manage", true},
		{"admin-p3-multi", "send_message", true},
		{"admin-p3-multi", "read", true},
		// Member: send, read but not manage
		{"member-p3-multi", "delete", false},
		{"member-p3-multi", "manage", false},
		{"member-p3-multi", "send_message", true},
		{"member-p3-multi", "read", true},
		// Viewer: read only
		{"viewer-p3-multi", "delete", false},
		{"viewer-p3-multi", "manage", false},
		{"viewer-p3-multi", "send_message", false},
		{"viewer-p3-multi", "read", true},
	}

	for _, tc := range expected {
		result, checkErr := adapter.Check(ctx, security.CheckRequest{
			Object:     room,
			Permission: tc.perm,
			Subject:    security.SubjectRef{ID: tc.user},
		})
		s.Require().NoError(checkErr)
		s.Equal(tc.allowed, result.Allowed, "%s %s should be %v", tc.user, tc.perm, tc.allowed)
	}
}

func (s *AuthorizerTestSuite) TestPlane3_File_Permissions() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	fileObj := security.ObjectRef{Namespace: "file", ID: "file-p3-perms"}

	tuples := []security.RelationTuple{
		{Object: fileObj, Relation: "owner", Subject: security.SubjectRef{ID: "owner-p3-file"}},
		{Object: fileObj, Relation: "editor", Subject: security.SubjectRef{ID: "editor-p3-file"}},
		{Object: fileObj, Relation: "viewer", Subject: security.SubjectRef{ID: "viewer-p3-file"}},
	}
	err := adapter.WriteTuples(ctx, tuples)
	s.Require().NoError(err)

	type userPerm struct {
		user    string
		perm    string
		allowed bool
	}

	expected := []userPerm{
		// Owner: all
		{"owner-p3-file", "read", true},
		{"owner-p3-file", "write", true},
		{"owner-p3-file", "delete", true},
		{"owner-p3-file", "share", true},
		// Editor: read + write
		{"editor-p3-file", "read", true},
		{"editor-p3-file", "write", true},
		{"editor-p3-file", "delete", false},
		{"editor-p3-file", "share", false},
		// Viewer: read only
		{"viewer-p3-file", "read", true},
		{"viewer-p3-file", "write", false},
		{"viewer-p3-file", "delete", false},
		{"viewer-p3-file", "share", false},
	}

	for _, tc := range expected {
		result, checkErr := adapter.Check(ctx, security.CheckRequest{
			Object:     fileObj,
			Permission: tc.perm,
			Subject:    security.SubjectRef{ID: tc.user},
		})
		s.Require().NoError(checkErr)
		s.Equal(tc.allowed, result.Allowed, "file %s %s should be %v", tc.user, tc.perm, tc.allowed)
	}
}

func (s *AuthorizerTestSuite) TestPlane3_File_CrossResourceIsolation() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	// Editor on file A, nothing on file B
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "file", ID: "file-iso-A"},
		Relation: "editor",
		Subject:  security.SubjectRef{ID: "user-p3-fileiso"},
	})
	s.Require().NoError(err)

	resultA, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "file", ID: "file-iso-A"},
		Permission: "write",
		Subject:    security.SubjectRef{ID: "user-p3-fileiso"},
	})
	s.Require().NoError(err)
	s.True(resultA.Allowed, "editor should write file A")

	resultB, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "file", ID: "file-iso-B"},
		Permission: "write",
		Subject:    security.SubjectRef{ID: "user-p3-fileiso"},
	})
	s.Require().NoError(err)
	s.False(resultB.Allowed, "should NOT write file B")
}

// ---------------------------------------------------------------------------
// ResourceAccessChecker integration tests
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestPlane3_ResourceAccessChecker_Check() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "chat_room")

	err := checker.Grant(ctx, "rac-room-1", "member", "user-rac-1")
	s.Require().NoError(err)

	// Check via claims context
	claimsCtxVal := claimsCtx("user-rac-1", "any-t", "any-p", "user")
	err = checker.Check(claimsCtxVal, "rac-room-1", "send_message")
	s.Require().NoError(err, "member should pass send_message check")

	err = checker.Check(claimsCtxVal, "rac-room-1", "manage")
	s.Require().Error(err, "member should fail manage check")
}

func (s *AuthorizerTestSuite) TestPlane3_ResourceAccessChecker_CheckSubject() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "chat_room")

	err := checker.Grant(ctx, "rac-room-2", "admin", "user-rac-2")
	s.Require().NoError(err)

	err = checker.CheckSubject(ctx, "rac-room-2", "invite", "user-rac-2")
	s.Require().NoError(err, "admin should have invite via CheckSubject")

	err = checker.CheckSubject(ctx, "rac-room-2", "delete", "user-rac-2")
	s.Require().Error(err, "admin should NOT have delete")
}

func (s *AuthorizerTestSuite) TestPlane3_ResourceAccessChecker_GrantAndRevoke() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "file")

	// Grant
	err := checker.Grant(ctx, "rac-file-1", "editor", "user-rac-3")
	s.Require().NoError(err)

	err = checker.CheckSubject(ctx, "rac-file-1", "write", "user-rac-3")
	s.Require().NoError(err, "editor should write after grant")

	// Revoke
	err = checker.Revoke(ctx, "rac-file-1", "editor", "user-rac-3")
	s.Require().NoError(err)

	err = checker.CheckSubject(ctx, "rac-file-1", "write", "user-rac-3")
	s.Require().Error(err, "should be denied after revoke")
}

func (s *AuthorizerTestSuite) TestPlane3_ResourceAccessChecker_NoClaims() {
	adapter := s.newAdapter(nil)
	checker := authorizer.NewResourceAccessChecker(adapter, "chat_room")

	err := checker.Check(context.Background(), "room-1", "read")
	s.Require().ErrorIs(err, authorizer.ErrInvalidSubject)
}

func (s *AuthorizerTestSuite) TestPlane3_ResourceAccessChecker_WithSubjectNamespace() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "chat_room",
		authorizer.WithResourceSubjectNamespace("custom"),
	)

	// Write tuple with custom namespace subject
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "chat_room", ID: "rac-room-ns"},
		Relation: "member",
		Subject:  security.SubjectRef{Namespace: "custom", ID: "user-rac-ns"},
	})
	s.Require().NoError(err)

	err = checker.CheckSubject(ctx, "rac-room-ns", "send_message", "user-rac-ns")
	s.Require().NoError(err)
}

// ---------------------------------------------------------------------------
// Contextual Constraints — Time
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestPlane3_TimeConstraint_AllowedDuringBusinessHours() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "chat_room",
		authorizer.WithConstraints(
			authorizer.TimeWindowConstraint(9, 17, time.UTC),
		),
	)

	err := checker.Grant(ctx, "time-room-1", "member", "user-time-1")
	s.Require().NoError(err)

	// 10:00 UTC — within business hours
	ctx10 := authorizer.WithCurrentTime(ctx, time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC))
	err = checker.CheckSubject(ctx10, "time-room-1", "send_message", "user-time-1")
	s.Require().NoError(err, "should be allowed at 10:00 UTC")
}

func (s *AuthorizerTestSuite) TestPlane3_TimeConstraint_DeniedOutsideBusinessHours() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "chat_room",
		authorizer.WithConstraints(
			authorizer.TimeWindowConstraint(9, 17, time.UTC),
		),
	)

	err := checker.Grant(ctx, "time-room-2", "member", "user-time-2")
	s.Require().NoError(err)

	// 22:00 UTC — outside business hours
	ctx22 := authorizer.WithCurrentTime(ctx, time.Date(2026, 3, 29, 22, 0, 0, 0, time.UTC))
	err = checker.CheckSubject(ctx22, "time-room-2", "send_message", "user-time-2")
	s.Require().Error(err, "should be denied at 22:00 UTC")

	var permErr *authorizer.PermissionDeniedError
	s.Require().ErrorAs(err, &permErr)
	s.Contains(permErr.Reason, "outside allowed window")
}

func (s *AuthorizerTestSuite) TestPlane3_TimeConstraint_BoundaryHours() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "chat_room",
		authorizer.WithConstraints(
			authorizer.TimeWindowConstraint(9, 17, time.UTC),
		),
	)

	err := checker.Grant(ctx, "time-room-3", "member", "user-time-3")
	s.Require().NoError(err)

	// Exactly 09:00 — first allowed hour
	ctx9 := authorizer.WithCurrentTime(ctx, time.Date(2026, 3, 29, 9, 0, 0, 0, time.UTC))
	err = checker.CheckSubject(ctx9, "time-room-3", "send_message", "user-time-3")
	s.Require().NoError(err, "should be allowed at start boundary (09:00)")

	// Exactly 16:59 — last allowed minute
	ctx16 := authorizer.WithCurrentTime(ctx, time.Date(2026, 3, 29, 16, 59, 0, 0, time.UTC))
	err = checker.CheckSubject(ctx16, "time-room-3", "send_message", "user-time-3")
	s.Require().NoError(err, "should be allowed at 16:59")

	// Exactly 17:00 — first denied hour
	ctx17 := authorizer.WithCurrentTime(ctx, time.Date(2026, 3, 29, 17, 0, 0, 0, time.UTC))
	err = checker.CheckSubject(ctx17, "time-room-3", "send_message", "user-time-3")
	s.Require().Error(err, "should be denied at end boundary (17:00)")

	// Exactly 08:59 — just before allowed
	ctx8 := authorizer.WithCurrentTime(ctx, time.Date(2026, 3, 29, 8, 59, 0, 0, time.UTC))
	err = checker.CheckSubject(ctx8, "time-room-3", "send_message", "user-time-3")
	s.Require().Error(err, "should be denied at 08:59")
}

func (s *AuthorizerTestSuite) TestPlane3_TimeConstraint_MidnightWrap() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	// Night shift: 22:00-06:00
	checker := authorizer.NewResourceAccessChecker(adapter, "chat_room",
		authorizer.WithConstraints(
			authorizer.TimeWindowConstraint(22, 6, time.UTC),
		),
	)

	err := checker.Grant(ctx, "time-room-wrap", "member", "user-time-wrap")
	s.Require().NoError(err)

	tests := []struct {
		hour    int
		allowed bool
	}{
		{23, true},  // 23:00 — within night shift
		{0, true},   // 00:00 — after midnight, still in window
		{3, true},   // 03:00 — middle of night shift
		{5, true},   // 05:00 — last allowed hour
		{6, false},  // 06:00 — first denied hour
		{12, false}, // 12:00 — midday, outside window
		{21, false}, // 21:00 — just before window
		{22, true},  // 22:00 — start of window
	}

	for _, tc := range tests {
		ctxTime := authorizer.WithCurrentTime(ctx,
			time.Date(2026, 3, 29, tc.hour, 0, 0, 0, time.UTC))
		checkErr := checker.CheckSubject(ctxTime, "time-room-wrap", "send_message", "user-time-wrap")
		if tc.allowed {
			s.Require().NoError(checkErr, "hour %d should be allowed", tc.hour)
		} else {
			s.Require().Error(checkErr, "hour %d should be denied", tc.hour)
		}
	}
}

func (s *AuthorizerTestSuite) TestPlane3_TimeConstraint_WithTimezone() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	nairobi, err := time.LoadLocation("Africa/Nairobi")
	s.Require().NoError(err)

	// Business hours 9-17 in Nairobi (UTC+3)
	checker := authorizer.NewResourceAccessChecker(adapter, "chat_room",
		authorizer.WithConstraints(
			authorizer.TimeWindowConstraint(9, 17, nairobi),
		),
	)

	err = checker.Grant(ctx, "time-room-tz", "member", "user-time-tz")
	s.Require().NoError(err)

	// 10:00 UTC = 13:00 Nairobi → allowed
	ctx10utc := authorizer.WithCurrentTime(ctx, time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC))
	err = checker.CheckSubject(ctx10utc, "time-room-tz", "send_message", "user-time-tz")
	s.Require().NoError(err, "10:00 UTC = 13:00 Nairobi, should be allowed")

	// 04:00 UTC = 07:00 Nairobi → denied (before 09:00 Nairobi)
	ctx4utc := authorizer.WithCurrentTime(ctx, time.Date(2026, 3, 29, 4, 0, 0, 0, time.UTC))
	err = checker.CheckSubject(ctx4utc, "time-room-tz", "send_message", "user-time-tz")
	s.Require().Error(err, "04:00 UTC = 07:00 Nairobi, should be denied")

	// 15:00 UTC = 18:00 Nairobi → denied (after 17:00 Nairobi)
	ctx15utc := authorizer.WithCurrentTime(ctx, time.Date(2026, 3, 29, 15, 0, 0, 0, time.UTC))
	err = checker.CheckSubject(ctx15utc, "time-room-tz", "send_message", "user-time-tz")
	s.Require().Error(err, "15:00 UTC = 18:00 Nairobi, should be denied")
}

func (s *AuthorizerTestSuite) TestPlane3_TimeConstraint_NoPermissionStillDenied() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	// No constraints but user has no Keto relation
	checker := authorizer.NewResourceAccessChecker(adapter, "chat_room")

	err := checker.CheckSubject(ctx, "time-room-norel", "send_message", "nobody-time")
	s.Require().Error(err, "Keto denial should take precedence — constraints don't bypass relations")
}

// ---------------------------------------------------------------------------
// Contextual Constraints — Location
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestPlane3_LocationConstraint_AllowedLocation() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "file",
		authorizer.WithConstraints(
			authorizer.LocationConstraint("office-nairobi", "office-london"),
		),
	)

	err := checker.Grant(ctx, "loc-file-1", "editor", "user-loc-1")
	s.Require().NoError(err)

	ctxNairobi := authorizer.WithLocation(ctx, "office-nairobi")
	err = checker.CheckSubject(ctxNairobi, "loc-file-1", "write", "user-loc-1")
	s.Require().NoError(err, "should be allowed from Nairobi office")

	ctxLondon := authorizer.WithLocation(ctx, "office-london")
	err = checker.CheckSubject(ctxLondon, "loc-file-1", "write", "user-loc-1")
	s.Require().NoError(err, "should be allowed from London office")
}

func (s *AuthorizerTestSuite) TestPlane3_LocationConstraint_DeniedLocation() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "file",
		authorizer.WithConstraints(
			authorizer.LocationConstraint("office-nairobi"),
		),
	)

	err := checker.Grant(ctx, "loc-file-2", "editor", "user-loc-2")
	s.Require().NoError(err)

	ctxCafe := authorizer.WithLocation(ctx, "public-cafe")
	err = checker.CheckSubject(ctxCafe, "loc-file-2", "write", "user-loc-2")
	s.Require().Error(err, "should be denied from unapproved location")

	var permErr *authorizer.PermissionDeniedError
	s.Require().ErrorAs(err, &permErr)
	s.Contains(permErr.Reason, "not in the allowed set")
}

func (s *AuthorizerTestSuite) TestPlane3_LocationConstraint_MissingLocation() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "file",
		authorizer.WithConstraints(
			authorizer.LocationConstraint("office-nairobi"),
		),
	)

	err := checker.Grant(ctx, "loc-file-3", "editor", "user-loc-3")
	s.Require().NoError(err)

	// No location in context at all
	err = checker.CheckSubject(ctx, "loc-file-3", "write", "user-loc-3")
	s.Require().Error(err, "should be denied when location is missing from context")

	var permErr *authorizer.PermissionDeniedError
	s.Require().ErrorAs(err, &permErr)
	s.Contains(permErr.Reason, "no location")
}

func (s *AuthorizerTestSuite) TestPlane3_LocationConstraint_NoPermissionStillDenied() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "file",
		authorizer.WithConstraints(
			authorizer.LocationConstraint("office-nairobi"),
		),
	)

	// User has NO Keto relation but is at the right location
	ctxNairobi := authorizer.WithLocation(ctx, "office-nairobi")
	err := checker.CheckSubject(ctxNairobi, "loc-file-norel", "write", "nobody-loc")
	s.Require().Error(err, "Keto denial takes precedence — location alone is not enough")
}

// ---------------------------------------------------------------------------
// Contextual Constraints — Combined (time + location)
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestPlane3_CombinedConstraints_BothSatisfied() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "file",
		authorizer.WithConstraints(
			authorizer.TimeWindowConstraint(9, 17, time.UTC),
			authorizer.LocationConstraint("office-nairobi", "office-london"),
		),
	)

	err := checker.Grant(ctx, "combo-file-1", "editor", "user-combo-1")
	s.Require().NoError(err)

	// 10:00 UTC + office-nairobi → both constraints satisfied
	ctxOK := authorizer.WithCurrentTime(ctx, time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC))
	ctxOK = authorizer.WithLocation(ctxOK, "office-nairobi")
	err = checker.CheckSubject(ctxOK, "combo-file-1", "write", "user-combo-1")
	s.Require().NoError(err, "both constraints satisfied, should be allowed")
}

func (s *AuthorizerTestSuite) TestPlane3_CombinedConstraints_TimeDenied() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "file",
		authorizer.WithConstraints(
			authorizer.TimeWindowConstraint(9, 17, time.UTC),
			authorizer.LocationConstraint("office-nairobi"),
		),
	)

	err := checker.Grant(ctx, "combo-file-2", "editor", "user-combo-2")
	s.Require().NoError(err)

	// 22:00 UTC + office-nairobi → time constraint fails
	ctxBadTime := authorizer.WithCurrentTime(ctx, time.Date(2026, 3, 29, 22, 0, 0, 0, time.UTC))
	ctxBadTime = authorizer.WithLocation(ctxBadTime, "office-nairobi")
	err = checker.CheckSubject(ctxBadTime, "combo-file-2", "write", "user-combo-2")
	s.Require().Error(err, "time constraint fails, should be denied")
}

func (s *AuthorizerTestSuite) TestPlane3_CombinedConstraints_LocationDenied() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "file",
		authorizer.WithConstraints(
			authorizer.TimeWindowConstraint(9, 17, time.UTC),
			authorizer.LocationConstraint("office-nairobi"),
		),
	)

	err := checker.Grant(ctx, "combo-file-3", "editor", "user-combo-3")
	s.Require().NoError(err)

	// 10:00 UTC + public-cafe → location constraint fails
	ctxBadLoc := authorizer.WithCurrentTime(ctx, time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC))
	ctxBadLoc = authorizer.WithLocation(ctxBadLoc, "public-cafe")
	err = checker.CheckSubject(ctxBadLoc, "combo-file-3", "write", "user-combo-3")
	s.Require().Error(err, "location constraint fails, should be denied")
}

func (s *AuthorizerTestSuite) TestPlane3_CombinedConstraints_BothDenied() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "file",
		authorizer.WithConstraints(
			authorizer.TimeWindowConstraint(9, 17, time.UTC),
			authorizer.LocationConstraint("office-nairobi"),
		),
	)

	err := checker.Grant(ctx, "combo-file-4", "editor", "user-combo-4")
	s.Require().NoError(err)

	// 22:00 UTC + public-cafe → both constraints fail (first one short-circuits)
	ctxBadBoth := authorizer.WithCurrentTime(ctx, time.Date(2026, 3, 29, 22, 0, 0, 0, time.UTC))
	ctxBadBoth = authorizer.WithLocation(ctxBadBoth, "public-cafe")
	err = checker.CheckSubject(ctxBadBoth, "combo-file-4", "write", "user-combo-4")
	s.Require().Error(err, "both constraints fail, should be denied")
}

func (s *AuthorizerTestSuite) TestPlane3_NoConstraints_IgnoresContext() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	// Checker with no constraints — time/location in context are irrelevant
	checker := authorizer.NewResourceAccessChecker(adapter, "chat_room")

	err := checker.Grant(ctx, "noconst-room", "member", "user-noconst")
	s.Require().NoError(err)

	ctxLate := authorizer.WithCurrentTime(ctx, time.Date(2026, 3, 29, 23, 0, 0, 0, time.UTC))
	ctxLate = authorizer.WithLocation(ctxLate, "unknown-place")
	err = checker.CheckSubject(ctxLate, "noconst-room", "send_message", "user-noconst")
	s.Require().NoError(err, "no constraints configured, context values should be ignored")
}

func (s *AuthorizerTestSuite) TestPlane3_Constraints_ViaClaimsCheck() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "chat_room",
		authorizer.WithConstraints(
			authorizer.TimeWindowConstraint(9, 17, time.UTC),
			authorizer.LocationConstraint("office-nairobi"),
		),
	)

	err := checker.Grant(ctx, "claims-const-room", "member", "user-claims-const")
	s.Require().NoError(err)

	// Check via claims-based Check() method, not CheckSubject()
	claimsCtxVal := claimsCtx("user-claims-const", "t1", "p1", "user")
	claimsCtxVal = authorizer.WithCurrentTime(claimsCtxVal, time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC))
	claimsCtxVal = authorizer.WithLocation(claimsCtxVal, "office-nairobi")
	err = checker.Check(claimsCtxVal, "claims-const-room", "send_message")
	s.Require().NoError(err, "claims-based Check should also evaluate constraints")

	// Same user, bad time
	badCtx := claimsCtx("user-claims-const", "t1", "p1", "user")
	badCtx = authorizer.WithCurrentTime(badCtx, time.Date(2026, 3, 29, 23, 0, 0, 0, time.UTC))
	badCtx = authorizer.WithLocation(badCtx, "office-nairobi")
	err = checker.Check(badCtx, "claims-const-room", "send_message")
	s.Require().Error(err, "should be denied when time constraint fails via Check()")
}

// ---------------------------------------------------------------------------
// Constraint Validation and Edge Cases
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestConstraint_TimeWindowPanicsOnInvalidStartHour() {
	s.Panics(func() {
		authorizer.TimeWindowConstraint(-1, 17, time.UTC)
	}, "negative startHour should panic")

	s.Panics(func() {
		authorizer.TimeWindowConstraint(24, 17, time.UTC)
	}, "startHour > 23 should panic")
}

func (s *AuthorizerTestSuite) TestConstraint_TimeWindowPanicsOnInvalidEndHour() {
	s.Panics(func() {
		authorizer.TimeWindowConstraint(9, -1, time.UTC)
	}, "negative endHour should panic")

	s.Panics(func() {
		authorizer.TimeWindowConstraint(9, 24, time.UTC)
	}, "endHour > 23 should panic")
}

func (s *AuthorizerTestSuite) TestConstraint_TimeWindowPanicsOnEqualHours() {
	s.Panics(func() {
		authorizer.TimeWindowConstraint(12, 12, time.UTC)
	}, "startHour == endHour should panic")
}

func (s *AuthorizerTestSuite) TestConstraint_LocationPanicsOnEmpty() {
	s.Panics(func() {
		authorizer.LocationConstraint()
	}, "empty allowed list should panic")
}

func (s *AuthorizerTestSuite) TestConstraint_LocationCaseInsensitive() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "chat_room",
		authorizer.WithConstraints(
			authorizer.LocationConstraint("Office-Nairobi"),
		),
	)

	err := checker.Grant(ctx, "case-room", "member", "user-case")
	s.Require().NoError(err)

	// WithLocation normalizes to lowercase, LocationConstraint normalizes to lowercase
	ctxMixed := authorizer.WithLocation(ctx, "OFFICE-NAIROBI")
	err = checker.CheckSubject(ctxMixed, "case-room", "send_message", "user-case")
	s.Require().NoError(err, "location matching should be case-insensitive")
}

// ---------------------------------------------------------------------------
// AnyConstraint (OR combinator)
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestConstraint_AnyConstraint_FirstPasses() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	// Allow if EITHER office location OR business hours
	checker := authorizer.NewResourceAccessChecker(adapter, "chat_room",
		authorizer.WithConstraints(
			authorizer.AnyConstraint(
				authorizer.LocationConstraint("office-nairobi"),
				authorizer.TimeWindowConstraint(9, 17, time.UTC),
			),
		),
	)

	err := checker.Grant(ctx, "any-room-1", "member", "user-any-1")
	s.Require().NoError(err)

	// Right location, wrong time → passes (location satisfies OR)
	ctxLoc := authorizer.WithLocation(ctx, "office-nairobi")
	ctxLoc = authorizer.WithCurrentTime(ctxLoc, time.Date(2026, 3, 29, 23, 0, 0, 0, time.UTC))
	err = checker.CheckSubject(ctxLoc, "any-room-1", "send_message", "user-any-1")
	s.Require().NoError(err, "should pass: location satisfies the OR")
}

func (s *AuthorizerTestSuite) TestConstraint_AnyConstraint_SecondPasses() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "chat_room",
		authorizer.WithConstraints(
			authorizer.AnyConstraint(
				authorizer.LocationConstraint("office-nairobi"),
				authorizer.TimeWindowConstraint(9, 17, time.UTC),
			),
		),
	)

	err := checker.Grant(ctx, "any-room-2", "member", "user-any-2")
	s.Require().NoError(err)

	// Wrong location, right time → passes (time satisfies OR)
	ctxTime := authorizer.WithLocation(ctx, "home")
	ctxTime = authorizer.WithCurrentTime(ctxTime, time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC))
	err = checker.CheckSubject(ctxTime, "any-room-2", "send_message", "user-any-2")
	s.Require().NoError(err, "should pass: time satisfies the OR")
}

func (s *AuthorizerTestSuite) TestConstraint_AnyConstraint_AllFail() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	checker := authorizer.NewResourceAccessChecker(adapter, "chat_room",
		authorizer.WithConstraints(
			authorizer.AnyConstraint(
				authorizer.LocationConstraint("office-nairobi"),
				authorizer.TimeWindowConstraint(9, 17, time.UTC),
			),
		),
	)

	err := checker.Grant(ctx, "any-room-3", "member", "user-any-3")
	s.Require().NoError(err)

	// Wrong location + wrong time → denied
	ctxBad := authorizer.WithLocation(ctx, "home")
	ctxBad = authorizer.WithCurrentTime(ctxBad, time.Date(2026, 3, 29, 23, 0, 0, 0, time.UTC))
	err = checker.CheckSubject(ctxBad, "any-room-3", "send_message", "user-any-3")
	s.Require().Error(err, "should be denied: neither constraint passes")
}

// ---------------------------------------------------------------------------
// Per-Permission Constraints
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestConstraint_PerPermission_DeleteNeedsLocation() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	// Only "delete" requires location; "read" has no constraints
	checker := authorizer.NewResourceAccessChecker(adapter, "chat_room",
		authorizer.WithPermissionConstraints("delete",
			authorizer.LocationConstraint("office-nairobi"),
		),
	)

	err := checker.Grant(ctx, "pp-room", "owner", "user-pp")
	s.Require().NoError(err)

	// Read: no constraints → allowed without location
	err = checker.CheckSubject(ctx, "pp-room", "read", "user-pp")
	s.Require().NoError(err, "read should pass without location constraint")

	// Delete without location → denied
	err = checker.CheckSubject(ctx, "pp-room", "delete", "user-pp")
	s.Require().Error(err, "delete should require location")

	// Delete with correct location → allowed
	ctxLoc := authorizer.WithLocation(ctx, "office-nairobi")
	err = checker.CheckSubject(ctxLoc, "pp-room", "delete", "user-pp")
	s.Require().NoError(err, "delete from office should pass")
}

func (s *AuthorizerTestSuite) TestConstraint_PerPermission_GlobalPlusSpecific() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	// Global: business hours. delete-specific: also needs location.
	checker := authorizer.NewResourceAccessChecker(adapter, "file",
		authorizer.WithConstraints(
			authorizer.TimeWindowConstraint(9, 17, time.UTC),
		),
		authorizer.WithPermissionConstraints("delete",
			authorizer.LocationConstraint("office-nairobi"),
		),
	)

	err := checker.Grant(ctx, "pp-file", "owner", "user-pp-file")
	s.Require().NoError(err)

	// Read at 10:00 without location → allowed (global passes, no perm-specific)
	ctxRead := authorizer.WithCurrentTime(ctx, time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC))
	err = checker.CheckSubject(ctxRead, "pp-file", "read", "user-pp-file")
	s.Require().NoError(err, "read during business hours should pass")

	// Delete at 10:00 without location → denied (global passes, perm-specific fails)
	err = checker.CheckSubject(ctxRead, "pp-file", "delete", "user-pp-file")
	s.Require().Error(err, "delete needs location even during business hours")

	// Delete at 10:00 with location → allowed
	ctxDel := authorizer.WithCurrentTime(ctx, time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC))
	ctxDel = authorizer.WithLocation(ctxDel, "office-nairobi")
	err = checker.CheckSubject(ctxDel, "pp-file", "delete", "user-pp-file")
	s.Require().NoError(err, "delete during business hours from office should pass")

	// Delete at 22:00 with location → denied (global time fails first)
	ctxLate := authorizer.WithCurrentTime(ctx, time.Date(2026, 3, 29, 22, 0, 0, 0, time.UTC))
	ctxLate = authorizer.WithLocation(ctxLate, "office-nairobi")
	err = checker.CheckSubject(ctxLate, "pp-file", "delete", "user-pp-file")
	s.Require().Error(err, "delete outside business hours denied even with location")
}

// ---------------------------------------------------------------------------
// Constraints on TenancyAccessChecker and FunctionChecker
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestConstraint_TenancyChecker_WithConstraints() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("const-ten-t", "const-ten-p")
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
		Relation: "member",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: "user-const-ten"},
	})
	s.Require().NoError(err)

	checker := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access",
		authorizer.WithTenancyConstraints(
			authorizer.TimeWindowConstraint(9, 17, time.UTC),
		),
	)

	// During business hours → allowed
	goodCtx := claimsCtx("user-const-ten", "const-ten-t", "const-ten-p", "user")
	goodCtx = authorizer.WithCurrentTime(goodCtx, time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC))
	err = checker.CheckAccess(goodCtx)
	s.Require().NoError(err, "should pass during business hours")

	// Outside business hours → denied by constraint
	badCtx := claimsCtx("user-const-ten", "const-ten-t", "const-ten-p", "user")
	badCtx = authorizer.WithCurrentTime(badCtx, time.Date(2026, 3, 29, 22, 0, 0, 0, time.UTC))
	err = checker.CheckAccess(badCtx)
	s.Require().Error(err, "should be denied outside business hours")

	var permErr *authorizer.PermissionDeniedError
	s.Require().ErrorAs(err, &permErr)
	s.Contains(permErr.Reason, "outside allowed window")
}

func (s *AuthorizerTestSuite) TestConstraint_FunctionChecker_WithConstraints() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("const-fn-t", "const-fn-p")
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tp},
		Relation: "admin",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: "user-const-fn"},
	})
	s.Require().NoError(err)

	checker := authorizer.NewFunctionChecker(adapter, "service_tenancy",
		authorizer.WithFunctionConstraints(
			authorizer.LocationConstraint("office-nairobi"),
		),
	)

	// From office → allowed
	goodCtx := claimsCtx("user-const-fn", "const-fn-t", "const-fn-p", "user")
	goodCtx = authorizer.WithLocation(goodCtx, "office-nairobi")
	err = checker.Check(goodCtx, "create")
	s.Require().NoError(err, "admin from office should have create")

	// From home → denied by constraint
	badCtx := claimsCtx("user-const-fn", "const-fn-t", "const-fn-p", "user")
	badCtx = authorizer.WithLocation(badCtx, "home")
	err = checker.Check(badCtx, "create")
	s.Require().Error(err, "admin from home should be denied by location constraint")
}

func (s *AuthorizerTestSuite) TestConstraint_FunctionChecker_PerPermission() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("const-fnpp-t", "const-fnpp-p")
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tp},
		Relation: "owner",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: "user-const-fnpp"},
	})
	s.Require().NoError(err)

	checker := authorizer.NewFunctionChecker(adapter, "service_tenancy",
		authorizer.WithFunctionPermissionConstraints("delete",
			authorizer.LocationConstraint("office-nairobi"),
		),
	)

	claimsVal := claimsCtx("user-const-fnpp", "const-fnpp-t", "const-fnpp-p", "user")

	// read: no perm-specific constraints → allowed without location
	err = checker.Check(claimsVal, "read")
	s.Require().NoError(err, "read should pass without location")

	// delete: perm-specific location constraint → denied without location
	err = checker.Check(claimsVal, "delete")
	s.Require().Error(err, "delete should require location")

	// delete with location → allowed
	locCtx := authorizer.WithLocation(claimsVal, "office-nairobi")
	err = checker.Check(locCtx, "delete")
	s.Require().NoError(err, "delete from office should pass")
}

// ---------------------------------------------------------------------------
// Panic Recovery
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestConstraint_PanicRecovery() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	panicConstraint := func(_ context.Context) error {
		panic("unexpected nil pointer")
	}

	checker := authorizer.NewResourceAccessChecker(adapter, "chat_room",
		authorizer.WithConstraints(panicConstraint),
	)

	err := checker.Grant(ctx, "panic-room", "member", "user-panic")
	s.Require().NoError(err)

	// Should NOT panic — should return a PermissionDeniedError
	err = checker.CheckSubject(ctx, "panic-room", "send_message", "user-panic")
	s.Require().Error(err, "panicking constraint should return error, not crash")

	var permErr *authorizer.PermissionDeniedError
	s.Require().ErrorAs(err, &permErr)
	s.Contains(permErr.Reason, "constraint panic")
}

// ===========================================================================
// Cross-Plane Scenarios
//
// End-to-end scenarios that exercise all three planes together.
// ===========================================================================

func (s *AuthorizerTestSuite) TestCrossPlane_FullAccessFlow() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tenantID := "xp-tenant"
	partitionID := "xp-partition"
	userID := "alice-xp"
	tp := tenancyPath(tenantID, partitionID)

	// Plane 1: Grant data access
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
		Relation: "member",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: userID},
	})
	s.Require().NoError(err)

	// Plane 2: Grant admin role in service_tenancy
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tp},
		Relation: "admin",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: userID},
	})
	s.Require().NoError(err)

	// Plane 3: Grant room membership
	roomID := "xp-room-1"
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "chat_room", ID: roomID},
		Relation: "member",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: userID},
	})
	s.Require().NoError(err)

	// Verify all three planes using checkers
	claimsCtxVal := claimsCtx(userID, tenantID, partitionID, "user")

	// Plane 1: TenancyAccessChecker
	tenancyChecker := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access")
	err = tenancyChecker.CheckAccess(claimsCtxVal)
	s.Require().NoError(err, "Plane 1: should have data access")

	// Plane 2: FunctionChecker
	funcChecker := authorizer.NewFunctionChecker(adapter, "service_tenancy")
	err = funcChecker.Check(claimsCtxVal, "create")
	s.Require().NoError(err, "Plane 2: admin should have create")
	err = funcChecker.Check(claimsCtxVal, "delete")
	s.Require().Error(err, "Plane 2: admin should NOT have delete")

	// Plane 3: ResourceAccessChecker
	roomChecker := authorizer.NewResourceAccessChecker(adapter, "chat_room")
	err = roomChecker.Check(claimsCtxVal, roomID, "send_message")
	s.Require().NoError(err, "Plane 3: member should send messages")
	err = roomChecker.Check(claimsCtxVal, roomID, "manage")
	s.Require().Error(err, "Plane 3: member should NOT manage")
}

func (s *AuthorizerTestSuite) TestCrossPlane_ServiceAccount() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tenantID := "xp-svc-t"
	partitionID := "xp-svc-p"
	svcID := "svc-xp-bot"
	tp := tenancyPath(tenantID, partitionID)

	// Plane 1: Service access
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
		Relation: "service",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: svcID},
	})
	s.Require().NoError(err)

	// Plane 2: Explicit grants only (no role, least privilege)
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tp},
		Relation: "granted_read",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: svcID},
	})
	s.Require().NoError(err)

	// Verify
	claimsCtxVal := claimsCtx(svcID, tenantID, partitionID, "internal")

	// Plane 1: Service access
	tenancyChecker := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access")
	err = tenancyChecker.CheckAccess(claimsCtxVal)
	s.Require().NoError(err, "Plane 1: service account should pass")

	// Plane 2: Only read, not create
	funcChecker := authorizer.NewFunctionChecker(adapter, "service_tenancy")
	err = funcChecker.Check(claimsCtxVal, "read")
	s.Require().NoError(err, "Plane 2: should have read via granted_read")
	err = funcChecker.Check(claimsCtxVal, "create")
	s.Require().Error(err, "Plane 2: should NOT have create without grant")
}

func (s *AuthorizerTestSuite) TestCrossPlane_UserWithMultiplePartitions() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	userID := "alice-xp-mp"
	tenantID := "xp-mp-t"

	// Two partitions with different roles
	tpDev := tenancyPath(tenantID, "dev")
	tpProd := tenancyPath(tenantID, "prod")

	// Plane 1: Access to both partitions
	for _, tp := range []string{tpDev, tpProd} {
		err := adapter.WriteTuple(ctx, security.RelationTuple{
			Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
			Relation: "member",
			Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: userID},
		})
		s.Require().NoError(err)
	}

	// Plane 2: Owner in dev, member in prod
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tpDev},
		Relation: "owner",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: userID},
	})
	s.Require().NoError(err)
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tpProd},
		Relation: "member",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: userID},
	})
	s.Require().NoError(err)

	funcChecker := authorizer.NewFunctionChecker(adapter, "service_tenancy")

	// Dev partition: owner can delete
	devCtx := claimsCtx(userID, tenantID, "dev", "user")
	err = funcChecker.Check(devCtx, "delete")
	s.Require().NoError(err, "owner should delete in dev")

	// Prod partition: member cannot delete
	prodCtx := claimsCtx(userID, tenantID, "prod", "user")
	err = funcChecker.Check(prodCtx, "delete")
	s.Require().Error(err, "member should NOT delete in prod")

	// Prod partition: member can read
	err = funcChecker.Check(prodCtx, "read")
	s.Require().NoError(err, "member should read in prod")
}

func (s *AuthorizerTestSuite) TestCrossPlane_NoDataAccessBlocksFunctionCheck() {
	adapter := s.newAdapter(nil)

	// Only grant Plane 2 role, but no Plane 1 access
	ctx := s.T().Context()
	tp := tenancyPath("xp-nodata-t", "xp-nodata-p")
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tp},
		Relation: "owner",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: "user-xp-nodata"},
	})
	s.Require().NoError(err)

	claimsCtxVal := claimsCtx("user-xp-nodata", "xp-nodata-t", "xp-nodata-p", "user")

	// Plane 1: Should be denied (no tenancy_access tuple)
	tenancyChecker := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access")
	err = tenancyChecker.CheckAccess(claimsCtxVal)
	s.Require().Error(err, "should fail Plane 1 check without data access")

	// Plane 2: Would pass IF the caller got past Plane 1
	funcChecker := authorizer.NewFunctionChecker(adapter, "service_tenancy")
	err = funcChecker.Check(claimsCtxVal, "create")
	s.Require().NoError(err, "Plane 2 check itself passes (it's up to interceptors to enforce ordering)")
}

func (s *AuthorizerTestSuite) TestCrossPlane_ResourceAccessAcrossNamespaces() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	userID := "alice-xp-xns"

	// Room in chat_room namespace
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "chat_room", ID: "xns-room"},
		Relation: "admin",
		Subject:  security.SubjectRef{ID: userID},
	})
	s.Require().NoError(err)

	// File in file namespace
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "file", ID: "xns-file"},
		Relation: "viewer",
		Subject:  security.SubjectRef{ID: userID},
	})
	s.Require().NoError(err)

	roomChecker := authorizer.NewResourceAccessChecker(adapter, "chat_room")
	fileChecker := authorizer.NewResourceAccessChecker(adapter, "file")

	// Room: admin can manage
	err = roomChecker.CheckSubject(ctx, "xns-room", "manage", userID)
	s.Require().NoError(err)

	// File: viewer can read but not write
	err = fileChecker.CheckSubject(ctx, "xns-file", "read", userID)
	s.Require().NoError(err)
	err = fileChecker.CheckSubject(ctx, "xns-file", "write", userID)
	s.Require().Error(err, "viewer should not write file")

	// Cross-namespace: room admin should NOT have file write
	err = fileChecker.CheckSubject(ctx, "xns-room", "write", userID)
	s.Require().Error(err, "room permissions should not leak to file namespace")
}

func (s *AuthorizerTestSuite) TestCrossPlane_BatchCheckAcrossPlanes() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	userID := "alice-xp-batch"
	tp := tenancyPath("xp-batch-t", "xp-batch-p")

	// Seed all three planes
	tuples := []security.RelationTuple{
		{
			Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
			Relation: "member",
			Subject:  security.SubjectRef{ID: userID},
		},
		{
			Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tp},
			Relation: "member",
			Subject:  security.SubjectRef{ID: userID},
		},
		{
			Object:   security.ObjectRef{Namespace: "chat_room", ID: "xp-batch-room"},
			Relation: "viewer",
			Subject:  security.SubjectRef{ID: userID},
		},
	}
	err := adapter.WriteTuples(ctx, tuples)
	s.Require().NoError(err)

	// Single BatchCheck covering all three planes
	checks := []security.CheckRequest{
		// Plane 1
		{
			Object:     security.ObjectRef{Namespace: "tenancy_access", ID: tp},
			Permission: "member",
			Subject:    security.SubjectRef{ID: userID},
		},
		// Plane 2
		{
			Object:     security.ObjectRef{Namespace: "service_tenancy", ID: tp},
			Permission: "read",
			Subject:    security.SubjectRef{ID: userID},
		},
		{
			Object:     security.ObjectRef{Namespace: "service_tenancy", ID: tp},
			Permission: "create",
			Subject:    security.SubjectRef{ID: userID},
		},
		// Plane 3
		{
			Object:     security.ObjectRef{Namespace: "chat_room", ID: "xp-batch-room"},
			Permission: "read",
			Subject:    security.SubjectRef{ID: userID},
		},
		{
			Object:     security.ObjectRef{Namespace: "chat_room", ID: "xp-batch-room"},
			Permission: "send_message",
			Subject:    security.SubjectRef{ID: userID},
		},
	}

	results, err := adapter.BatchCheck(ctx, checks)
	s.Require().NoError(err)
	s.Require().Len(results, 5)

	expected := []struct {
		desc    string
		allowed bool
	}{
		{"P1: member", true},
		{"P2: member can read", true},
		{"P2: member cannot create", false},
		{"P3: viewer can read", true},
		{"P3: viewer cannot send_message", false},
	}

	for i, e := range expected {
		s.Equal(e.allowed, results[i].Allowed, e.desc)
	}
}

func (s *AuthorizerTestSuite) TestCrossPlane_ListRelationsAcrossPlanes() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tp := tenancyPath("xp-list-t", "xp-list-p")

	// Seed tuples in all planes
	tuples := []security.RelationTuple{
		{
			Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
			Relation: "member",
			Subject:  security.SubjectRef{ID: "user-xp-list1"},
		},
		{
			Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
			Relation: "service",
			Subject:  security.SubjectRef{ID: "svc-xp-list1"},
		},
	}
	err := adapter.WriteTuples(ctx, tuples)
	s.Require().NoError(err)

	listed, err := adapter.ListRelations(ctx, security.ObjectRef{Namespace: "tenancy_access", ID: tp})
	s.Require().NoError(err)
	s.Len(listed, 2)

	relations := make(map[string]string)
	for _, t := range listed {
		relations[t.Subject.ID] = t.Relation
	}
	s.Equal("member", relations["user-xp-list1"])
	s.Equal("service", relations["svc-xp-list1"])
}

func (s *AuthorizerTestSuite) TestCrossPlane_FullTenantOnboarding() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tenantID := "xp-onboard-t"
	partitionID := "xp-onboard-p"
	tp := tenancyPath(tenantID, partitionID)

	ownerID := "owner-xp-onboard"
	adminID := "admin-xp-onboard"
	memberID := "member-xp-onboard"
	svcID := "svc-xp-onboard"

	// Plane 1: data access for all
	for _, uid := range []string{ownerID, adminID, memberID} {
		err := adapter.WriteTuple(ctx, security.RelationTuple{
			Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
			Relation: "member",
			Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: uid},
		})
		s.Require().NoError(err)
	}
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "tenancy_access", ID: tp},
		Relation: "service",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: svcID},
	})
	s.Require().NoError(err)

	// Plane 2: roles
	roles := []struct {
		role string
		uid  string
	}{
		{"owner", ownerID},
		{"admin", adminID},
		{"member", memberID},
	}
	for _, r := range roles {
		writeErr := adapter.WriteTuple(ctx, security.RelationTuple{
			Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tp},
			Relation: r.role,
			Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: r.uid},
		})
		s.Require().NoError(writeErr)
	}

	// Plane 2: service account grants
	for _, grant := range []string{"granted_read", "granted_list"} {
		writeErr := adapter.WriteTuple(ctx, security.RelationTuple{
			Object:   security.ObjectRef{Namespace: "service_tenancy", ID: tp},
			Relation: grant,
			Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: svcID},
		})
		s.Require().NoError(writeErr)
	}

	// Plane 3: create a room with owner as room owner
	roomID := fmt.Sprintf("%s-general", partitionID)
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "chat_room", ID: roomID},
		Relation: "owner",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: ownerID},
	})
	s.Require().NoError(err)
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "chat_room", ID: roomID},
		Relation: "member",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: memberID},
	})
	s.Require().NoError(err)

	// Verify full access chain
	tenancyChecker := authorizer.NewTenancyAccessChecker(adapter, "tenancy_access")
	funcChecker := authorizer.NewFunctionChecker(adapter, "service_tenancy")
	roomChecker := authorizer.NewResourceAccessChecker(adapter, "chat_room")

	// Owner: full access across all planes
	ownerCtx := claimsCtx(ownerID, tenantID, partitionID, "user")
	s.Require().NoError(tenancyChecker.CheckAccess(ownerCtx))
	s.Require().NoError(funcChecker.Check(ownerCtx, "delete"))
	s.Require().NoError(roomChecker.Check(ownerCtx, roomID, "delete"))

	// Admin: data access + create but not delete (P2), no room access (P3)
	adminCtx := claimsCtx(adminID, tenantID, partitionID, "user")
	s.Require().NoError(tenancyChecker.CheckAccess(adminCtx))
	s.Require().NoError(funcChecker.Check(adminCtx, "create"))
	s.Require().Error(funcChecker.Check(adminCtx, "delete"))
	s.Require().Error(roomChecker.Check(adminCtx, roomID, "send_message"))

	// Member: data access + read (P2) + send_message in room (P3)
	memberCtx := claimsCtx(memberID, tenantID, partitionID, "user")
	s.Require().NoError(tenancyChecker.CheckAccess(memberCtx))
	s.Require().NoError(funcChecker.Check(memberCtx, "read"))
	s.Require().Error(funcChecker.Check(memberCtx, "create"))
	s.Require().NoError(roomChecker.Check(memberCtx, roomID, "send_message"))

	// Service: service access (P1) + read/list (P2), no room access
	svcCtx := claimsCtx(svcID, tenantID, partitionID, "internal")
	s.Require().NoError(tenancyChecker.CheckAccess(svcCtx))
	s.Require().NoError(funcChecker.Check(svcCtx, "read"))
	s.Require().Error(funcChecker.Check(svcCtx, "create"))
}
