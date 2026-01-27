package authorizer_test

import (
	"github.com/pitabwire/frame/security"
)

// ---------------------------------------------------------------------------
// RBAC: Direct role assignment via subject sets
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestRBACDirectRoleAssignment() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	// Create role: "editors" is a group object in namespace "default"
	// Grant role access to a document
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "default", ID: "rbac-doc-1"},
		Relation: "edit",
		Subject:  security.SubjectRef{Namespace: "default", ID: "rbac-editors", Relation: "member"},
	})
	s.Require().NoError(err)

	// Assign alice to the editors role
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "default", ID: "rbac-editors"},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "alice-rbac"},
	})
	s.Require().NoError(err)

	// Alice should have edit access to the document through her role
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "default", ID: "rbac-doc-1"},
		Permission: "edit",
		Subject:    security.SubjectRef{Namespace: "default", ID: "rbac-editors", Relation: "member"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "editors role should have edit access")

	// Non-member should be denied
	result, err = adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "default", ID: "rbac-doc-1"},
		Permission: "edit",
		Subject:    security.SubjectRef{ID: "outsider-rbac"},
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "non-member should be denied")
}

// ---------------------------------------------------------------------------
// RBAC: Multiple roles with different permissions
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestRBACMultipleRoles() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	doc := security.ObjectRef{Namespace: "default", ID: "rbac-multi-doc-1"}

	// "viewers" role can view
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   doc,
		Relation: "view",
		Subject:  security.SubjectRef{Namespace: "default", ID: "rbac-viewers", Relation: "member"},
	})
	s.Require().NoError(err)

	// "editors" role can edit
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   doc,
		Relation: "edit",
		Subject:  security.SubjectRef{Namespace: "default", ID: "rbac-multi-editors", Relation: "member"},
	})
	s.Require().NoError(err)

	// Bob is in both roles
	for _, role := range []string{"rbac-viewers", "rbac-multi-editors"} {
		err = adapter.WriteTuple(ctx, security.RelationTuple{
			Object:   security.ObjectRef{Namespace: "default", ID: role},
			Relation: "member",
			Subject:  security.SubjectRef{ID: "bob-rbac-multi"},
		})
		s.Require().NoError(err)
	}

	// Carol is only a viewer
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "default", ID: "rbac-viewers"},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "carol-rbac-multi"},
	})
	s.Require().NoError(err)

	// Batch check bob's capabilities
	bobChecks := []security.CheckRequest{
		{Object: doc, Permission: "view",
			Subject: security.SubjectRef{Namespace: "default", ID: "rbac-viewers", Relation: "member"}},
		{Object: doc, Permission: "edit",
			Subject: security.SubjectRef{Namespace: "default", ID: "rbac-multi-editors", Relation: "member"}},
	}
	results, err := adapter.BatchCheck(ctx, bobChecks)
	s.Require().NoError(err)
	s.Require().Len(results, 2)
	s.True(results[0].Allowed, "bob should view through viewers role")
	s.True(results[1].Allowed, "bob should edit through editors role")

	// Carol can view but not edit
	carolView, err := adapter.Check(ctx, security.CheckRequest{
		Object:     doc,
		Permission: "view",
		Subject:    security.SubjectRef{Namespace: "default", ID: "rbac-viewers", Relation: "member"},
	})
	s.Require().NoError(err)
	s.True(carolView.Allowed)

	carolEdit, err := adapter.Check(ctx, security.CheckRequest{
		Object:     doc,
		Permission: "edit",
		Subject:    security.SubjectRef{ID: "carol-rbac-multi"},
	})
	s.Require().NoError(err)
	s.False(carolEdit.Allowed, "carol should not have direct edit access")
}

// ---------------------------------------------------------------------------
// RBAC: Role revocation removes access
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestRBACRoleRevocation() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	role := security.ObjectRef{Namespace: "default", ID: "rbac-revoke-admins"}
	resource := security.ObjectRef{Namespace: "default", ID: "rbac-revoke-res-1"}

	// Grant admins delete permission on resource
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   resource,
		Relation: "delete",
		Subject:  security.SubjectRef{Namespace: "default", ID: role.ID, Relation: "member"},
	})
	s.Require().NoError(err)

	// Dan is an admin
	memberTuple := security.RelationTuple{
		Object:   role,
		Relation: "member",
		Subject:  security.SubjectRef{ID: "dan-rbac-revoke"},
	}
	err = adapter.WriteTuple(ctx, memberTuple)
	s.Require().NoError(err)

	// Verify Dan has delete access through role
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     resource,
		Permission: "delete",
		Subject:    security.SubjectRef{Namespace: "default", ID: role.ID, Relation: "member"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "dan should have delete access as admin")

	// Revoke Dan's admin role
	err = adapter.DeleteTuple(ctx, memberTuple)
	s.Require().NoError(err)

	// Verify Dan's role membership is gone
	tuples, err := adapter.ListRelations(ctx, role)
	s.Require().NoError(err)

	for _, t := range tuples {
		s.NotEqual("dan-rbac-revoke", t.Subject.ID,
			"dan should no longer be listed as role member")
	}
}

// ---------------------------------------------------------------------------
// RBAC: Organization-scoped roles
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestRBACOrganizationScoped() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	// Org A and Org B each have their own admin role
	orgAAdmin := security.ObjectRef{Namespace: "partition", ID: "orgA-admin"}
	orgBAdmin := security.ObjectRef{Namespace: "partition", ID: "orgB-admin"}

	orgAResource := security.ObjectRef{Namespace: "partition", ID: "orgA-settings"}
	orgBResource := security.ObjectRef{Namespace: "partition", ID: "orgB-settings"}

	// Grant org admins access to their org's settings
	for _, pair := range []struct {
		resource security.ObjectRef
		admin    security.ObjectRef
	}{
		{orgAResource, orgAAdmin},
		{orgBResource, orgBAdmin},
	} {
		err := adapter.WriteTuple(ctx, security.RelationTuple{
			Object:   pair.resource,
			Relation: "manage",
			Subject:  security.SubjectRef{Namespace: "partition", ID: pair.admin.ID, Relation: "member"},
		})
		s.Require().NoError(err)
	}

	// Eve is admin in org A only
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   orgAAdmin,
		Relation: "member",
		Subject:  security.SubjectRef{ID: "eve-rbac-org"},
	})
	s.Require().NoError(err)

	// Frank is admin in org B only
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   orgBAdmin,
		Relation: "member",
		Subject:  security.SubjectRef{ID: "frank-rbac-org"},
	})
	s.Require().NoError(err)

	// Eve can manage org A settings
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     orgAResource,
		Permission: "manage",
		Subject:    security.SubjectRef{Namespace: "partition", ID: orgAAdmin.ID, Relation: "member"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "eve should manage org A settings")

	// Eve cannot manage org B settings (different role)
	result, err = adapter.Check(ctx, security.CheckRequest{
		Object:     orgBResource,
		Permission: "manage",
		Subject:    security.SubjectRef{Namespace: "partition", ID: orgAAdmin.ID, Relation: "member"},
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "eve should not manage org B settings")

	// Frank can manage org B settings
	result, err = adapter.Check(ctx, security.CheckRequest{
		Object:     orgBResource,
		Permission: "manage",
		Subject:    security.SubjectRef{Namespace: "partition", ID: orgBAdmin.ID, Relation: "member"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "frank should manage org B settings")
}

// ---------------------------------------------------------------------------
// RBAC: Bulk role members
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestRBACBulkRoleMembers() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	role := security.ObjectRef{Namespace: "default", ID: "rbac-bulk-contributors"}
	repo := security.ObjectRef{Namespace: "default", ID: "rbac-bulk-repo-1"}

	// Grant contributors push access
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   repo,
		Relation: "push",
		Subject:  security.SubjectRef{Namespace: "default", ID: role.ID, Relation: "member"},
	})
	s.Require().NoError(err)

	// Add 5 users to the contributors role
	users := []string{"user-bulk-1", "user-bulk-2", "user-bulk-3", "user-bulk-4", "user-bulk-5"}
	for _, u := range users {
		err = adapter.WriteTuple(ctx, security.RelationTuple{
			Object:   role,
			Relation: "member",
			Subject:  security.SubjectRef{ID: u},
		})
		s.Require().NoError(err)
	}

	// All users should have push access through the role
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     repo,
		Permission: "push",
		Subject:    security.SubjectRef{Namespace: "default", ID: role.ID, Relation: "member"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "contributors should have push access")

	// Verify all 5 members are listed
	tuples, err := adapter.ListRelations(ctx, role)
	s.Require().NoError(err)
	s.Len(tuples, 5, "role should have 5 members")

	memberIDs := make(map[string]bool)
	for _, t := range tuples {
		memberIDs[t.Subject.ID] = true
	}
	for _, u := range users {
		s.True(memberIDs[u], "user %s should be in role", u)
	}
}

// ---------------------------------------------------------------------------
// RBAC: Resource isolation (role only grants access to specific resources)
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestRBACResourceIsolation() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	role := security.ObjectRef{Namespace: "default", ID: "rbac-iso-editors"}
	allowedDoc := security.ObjectRef{Namespace: "default", ID: "rbac-iso-allowed-doc"}
	forbiddenDoc := security.ObjectRef{Namespace: "default", ID: "rbac-iso-forbidden-doc"}

	// Role grants edit only on allowedDoc
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   allowedDoc,
		Relation: "edit",
		Subject:  security.SubjectRef{Namespace: "default", ID: role.ID, Relation: "member"},
	})
	s.Require().NoError(err)

	// Grace is an editor
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   role,
		Relation: "member",
		Subject:  security.SubjectRef{ID: "grace-rbac-iso"},
	})
	s.Require().NoError(err)

	// Check via batch: allowed on one doc, denied on the other
	checks := []security.CheckRequest{
		{
			Object:     allowedDoc,
			Permission: "edit",
			Subject:    security.SubjectRef{Namespace: "default", ID: role.ID, Relation: "member"},
		},
		{
			Object:     forbiddenDoc,
			Permission: "edit",
			Subject:    security.SubjectRef{Namespace: "default", ID: role.ID, Relation: "member"},
		},
	}

	results, err := adapter.BatchCheck(ctx, checks)
	s.Require().NoError(err)
	s.Require().Len(results, 2)
	s.True(results[0].Allowed, "editors should edit allowed doc")
	s.False(results[1].Allowed, "editors should not edit forbidden doc")
}

// ---------------------------------------------------------------------------
// RBAC: Multi-tenant isolation using partition namespace
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestRBACMultiTenantIsolation() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	// Tenant A resources and roles
	tenantARole := security.ObjectRef{Namespace: "partition", ID: "tenantA-users"}
	tenantAData := security.ObjectRef{Namespace: "partition", ID: "tenantA-data"}

	// Tenant B resources and roles
	tenantBRole := security.ObjectRef{Namespace: "partition", ID: "tenantB-users"}
	tenantBData := security.ObjectRef{Namespace: "partition", ID: "tenantB-data"}

	// Grant tenant roles access to their data
	for _, pair := range []struct {
		data security.ObjectRef
		role security.ObjectRef
	}{
		{tenantAData, tenantARole},
		{tenantBData, tenantBRole},
	} {
		err := adapter.WriteTuple(ctx, security.RelationTuple{
			Object:   pair.data,
			Relation: "read",
			Subject:  security.SubjectRef{Namespace: "partition", ID: pair.role.ID, Relation: "member"},
		})
		s.Require().NoError(err)
	}

	// Henry belongs to tenant A
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   tenantARole,
		Relation: "member",
		Subject:  security.SubjectRef{ID: "henry-mt"},
	})
	s.Require().NoError(err)

	// Henry can read tenant A data
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     tenantAData,
		Permission: "read",
		Subject:    security.SubjectRef{Namespace: "partition", ID: tenantARole.ID, Relation: "member"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "henry should read tenant A data")

	// Henry cannot read tenant B data (different role)
	result, err = adapter.Check(ctx, security.CheckRequest{
		Object:     tenantBData,
		Permission: "read",
		Subject:    security.SubjectRef{Namespace: "partition", ID: tenantARole.ID, Relation: "member"},
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "henry should not read tenant B data")
}

// ---------------------------------------------------------------------------
// RBAC: Role enumeration via ListRelations
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestRBACRoleEnumeration() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	// Create multiple roles and assign a user to some
	roles := []string{"rbac-enum-admins", "rbac-enum-editors", "rbac-enum-reviewers"}
	user := security.SubjectRef{ID: "ivy-rbac-enum"}

	for _, role := range roles {
		err := adapter.WriteTuple(ctx, security.RelationTuple{
			Object:   security.ObjectRef{Namespace: "default", ID: role},
			Relation: "member",
			Subject:  user,
		})
		s.Require().NoError(err)
	}

	// Enumerate Ivy's memberships using ListSubjectRelations
	ivyTuples, err := adapter.ListSubjectRelations(ctx, user, "default")
	s.Require().NoError(err)
	s.GreaterOrEqual(len(ivyTuples), 3, "ivy should have at least 3 role memberships")

	// Verify each role lists ivy as a member
	for _, role := range roles {
		members, listErr := adapter.ListRelations(ctx,
			security.ObjectRef{Namespace: "default", ID: role})
		s.Require().NoError(listErr)

		found := false
		for _, t := range members {
			if t.Subject.ID == user.ID && t.Relation == "member" {
				found = true
				break
			}
		}
		s.True(found, "ivy should be listed as member of %s", role)
	}
}

// ---------------------------------------------------------------------------
// RBAC: Permission capabilities discovery via BatchCheck
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestRBACCapabilitiesDiscovery() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	resource := security.ObjectRef{Namespace: "default", ID: "rbac-cap-project"}
	ownerRole := security.ObjectRef{Namespace: "default", ID: "rbac-cap-owners"}
	devRole := security.ObjectRef{Namespace: "default", ID: "rbac-cap-devs"}

	// Owner role gets all permissions
	for _, perm := range []string{"view", "edit", "delete", "manage", "deploy"} {
		err := adapter.WriteTuple(ctx, security.RelationTuple{
			Object:   resource,
			Relation: perm,
			Subject:  security.SubjectRef{Namespace: "default", ID: ownerRole.ID, Relation: "member"},
		})
		s.Require().NoError(err)
	}

	// Dev role gets view, edit, deploy only
	for _, perm := range []string{"view", "edit", "deploy"} {
		err := adapter.WriteTuple(ctx, security.RelationTuple{
			Object:   resource,
			Relation: perm,
			Subject:  security.SubjectRef{Namespace: "default", ID: devRole.ID, Relation: "member"},
		})
		s.Require().NoError(err)
	}

	// Jack is an owner, Karen is a dev
	for _, pair := range []struct {
		role security.ObjectRef
		user string
	}{
		{ownerRole, "jack-rbac-cap"},
		{devRole, "karen-rbac-cap"},
	} {
		err := adapter.WriteTuple(ctx, security.RelationTuple{
			Object:   pair.role,
			Relation: "member",
			Subject:  security.SubjectRef{ID: pair.user},
		})
		s.Require().NoError(err)
	}

	allPerms := []string{"view", "edit", "delete", "manage", "deploy"}

	// Discover Jack's capabilities (owner - should have all 5)
	ownerChecks := make([]security.CheckRequest, len(allPerms))
	for i, p := range allPerms {
		ownerChecks[i] = security.CheckRequest{
			Object:     resource,
			Permission: p,
			Subject:    security.SubjectRef{Namespace: "default", ID: ownerRole.ID, Relation: "member"},
		}
	}
	ownerResults, err := adapter.BatchCheck(ctx, ownerChecks)
	s.Require().NoError(err)
	s.Require().Len(ownerResults, 5)

	ownerCaps := make(map[string]bool)
	for i, p := range allPerms {
		ownerCaps[p] = ownerResults[i].Allowed
	}
	s.True(ownerCaps["view"])
	s.True(ownerCaps["edit"])
	s.True(ownerCaps["delete"])
	s.True(ownerCaps["manage"])
	s.True(ownerCaps["deploy"])

	// Discover Karen's capabilities (dev - view, edit, deploy only)
	devChecks := make([]security.CheckRequest, len(allPerms))
	for i, p := range allPerms {
		devChecks[i] = security.CheckRequest{
			Object:     resource,
			Permission: p,
			Subject:    security.SubjectRef{Namespace: "default", ID: devRole.ID, Relation: "member"},
		}
	}
	devResults, err := adapter.BatchCheck(ctx, devChecks)
	s.Require().NoError(err)
	s.Require().Len(devResults, 5)

	devCaps := make(map[string]bool)
	for i, p := range allPerms {
		devCaps[p] = devResults[i].Allowed
	}
	s.True(devCaps["view"])
	s.True(devCaps["edit"])
	s.False(devCaps["delete"], "devs should not delete")
	s.False(devCaps["manage"], "devs should not manage")
	s.True(devCaps["deploy"])
}

// ---------------------------------------------------------------------------
// RBAC: Role membership transfer
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestRBACRoleMembershipTransfer() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	role := security.ObjectRef{Namespace: "default", ID: "rbac-transfer-leads"}
	resource := security.ObjectRef{Namespace: "default", ID: "rbac-transfer-pipeline"}

	// Grant leads access to pipeline
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   resource,
		Relation: "approve",
		Subject:  security.SubjectRef{Namespace: "default", ID: role.ID, Relation: "member"},
	})
	s.Require().NoError(err)

	// Leo is lead
	leoMembership := security.RelationTuple{
		Object:   role,
		Relation: "member",
		Subject:  security.SubjectRef{ID: "leo-rbac-transfer"},
	}
	err = adapter.WriteTuple(ctx, leoMembership)
	s.Require().NoError(err)

	// Leo can approve
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     resource,
		Permission: "approve",
		Subject:    security.SubjectRef{Namespace: "default", ID: role.ID, Relation: "member"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed)

	// Transfer: remove Leo, add Mia
	err = adapter.DeleteTuple(ctx, leoMembership)
	s.Require().NoError(err)

	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   role,
		Relation: "member",
		Subject:  security.SubjectRef{ID: "mia-rbac-transfer"},
	})
	s.Require().NoError(err)

	// Verify role has exactly Mia
	members, err := adapter.ListRelations(ctx, role)
	s.Require().NoError(err)
	s.Require().Len(members, 1)
	s.Equal("mia-rbac-transfer", members[0].Subject.ID)
}

// ---------------------------------------------------------------------------
// RBAC: Cross-namespace role access
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestRBACCrossNamespaceAccess() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	// Role defined in "default" namespace
	role := security.ObjectRef{Namespace: "default", ID: "rbac-cross-superadmins"}

	// Resource in "partition" namespace
	resource := security.ObjectRef{Namespace: "partition", ID: "rbac-cross-global-config"}

	// Grant cross-namespace access
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   resource,
		Relation: "configure",
		Subject:  security.SubjectRef{Namespace: "default", ID: role.ID, Relation: "member"},
	})
	s.Require().NoError(err)

	// Nick is a superadmin
	err = adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   role,
		Relation: "member",
		Subject:  security.SubjectRef{ID: "nick-rbac-cross"},
	})
	s.Require().NoError(err)

	// Nick can configure the partition resource through the default-namespace role
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     resource,
		Permission: "configure",
		Subject:    security.SubjectRef{Namespace: "default", ID: role.ID, Relation: "member"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "superadmin should configure partition resource")

	// Direct check without role should be denied
	result, err = adapter.Check(ctx, security.CheckRequest{
		Object:     resource,
		Permission: "configure",
		Subject:    security.SubjectRef{ID: "nobody-rbac-cross"},
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "non-superadmin should be denied")
}
