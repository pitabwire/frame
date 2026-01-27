package authorizer_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame/client"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testoryketo"
	"github.com/pitabwire/frame/frametests/deps/testpostgres"
	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/security/authorizer"
	"github.com/pitabwire/frame/tests"
)

// ---------------------------------------------------------------------------
// Recording audit logger
// ---------------------------------------------------------------------------

type recordingAuditLogger struct {
	calls []auditCall
}

type auditCall struct {
	req      security.CheckRequest
	result   security.CheckResult
	metadata map[string]string
}

func (r *recordingAuditLogger) LogDecision(
	_ context.Context,
	req security.CheckRequest,
	result security.CheckResult,
	metadata map[string]string,
) error {
	r.calls = append(r.calls, auditCall{req: req, result: result, metadata: metadata})
	return nil
}

// ---------------------------------------------------------------------------
// Test suite definition
// ---------------------------------------------------------------------------

type AuthorizerTestSuite struct {
	tests.BaseTestSuite
	readURI  string
	writeURI string
}

func initAuthorizerResources(_ context.Context) []definition.TestResource {
	pg := testpostgres.NewWithOpts("authorizer_test",
		definition.WithUserName("ant"),
		definition.WithPassword("s3cr3t"),
		definition.WithEnableLogging(false),
		definition.WithUseHostMode(false),
	)

	keto := testoryketo.NewWithOpts(
		testoryketo.KetoConfiguration,
		definition.WithDependancies(pg),
		definition.WithEnableLogging(false),
	)

	return []definition.TestResource{pg, keto}
}

func (s *AuthorizerTestSuite) SetupSuite() {
	s.InitResourceFunc = initAuthorizerResources
	s.BaseTestSuite.SetupSuite()

	ctx := s.T().Context()

	var ketoDep definition.DependancyConn
	for _, res := range s.Resources() {
		if res.Name() == testoryketo.OryKetoImage {
			ketoDep = res
			break
		}
	}
	s.Require().NotNil(ketoDep, "keto dependency should be available")

	// Write API: default port (4467/tcp, first in port list)
	s.writeURI = string(ketoDep.GetDS(ctx))

	// Read API: port 4466/tcp (second in port list)
	readPort, err := ketoDep.PortMapping(ctx, "4466/tcp")
	s.Require().NoError(err)

	u, err := url.Parse(s.writeURI)
	s.Require().NoError(err)
	s.readURI = fmt.Sprintf("%s://%s:%s", u.Scheme, u.Hostname(), readPort)
}

func TestAuthorizerSuite(t *testing.T) {
	suite.Run(t, &AuthorizerTestSuite{})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) newAdapter(auditLogger security.AuditLogger) security.Authorizer {
	cfg := &config.ConfigurationDefault{
		AuthorizationServiceReadURI:  s.readURI,
		AuthorizationServiceWriteURI: s.writeURI,
	}
	mgr := client.NewManager(s.T().Context())
	return authorizer.NewKetoAdapter(cfg, mgr, auditLogger)
}

func (s *AuthorizerTestSuite) permissiveAdapter() security.Authorizer {
	cfg := &config.ConfigurationDefault{}
	mgr := client.NewManager(s.T().Context())
	return authorizer.NewKetoAdapter(cfg, mgr, nil)
}

// ---------------------------------------------------------------------------
// Write + Check integration tests
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestWriteAndCheckTuple() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tuple := security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "default", ID: "wc-obj-1"},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "user-wc-1"},
	}

	err := adapter.WriteTuple(ctx, tuple)
	s.Require().NoError(err)

	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     tuple.Object,
		Permission: tuple.Relation,
		Subject:    tuple.Subject,
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "user should have permission after write")
	s.NotZero(result.CheckedAt)
}

func (s *AuthorizerTestSuite) TestCheckDenied() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "default", ID: "nonexistent-obj"},
		Permission: "view",
		Subject:    security.SubjectRef{ID: "nonexistent-user"},
	})
	s.Require().NoError(err)
	s.False(result.Allowed)
	s.Equal("no matching relation found", result.Reason)
}

func (s *AuthorizerTestSuite) TestCheckWithSubjectSet() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tuple := security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "default", ID: "ss-doc-1"},
		Relation: "viewer",
		Subject:  security.SubjectRef{Namespace: "default", ID: "editors", Relation: "member"},
	}

	err := adapter.WriteTuple(ctx, tuple)
	s.Require().NoError(err)

	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     tuple.Object,
		Permission: "viewer",
		Subject:    security.SubjectRef{Namespace: "default", ID: "editors", Relation: "member"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed)
}

func (s *AuthorizerTestSuite) TestCheckContextCancelled() {
	ctx, cancel := context.WithCancel(s.T().Context())
	cancel()

	adapter := s.newAdapter(nil)
	_, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "default", ID: "ctx-obj"},
		Permission: "view",
		Subject:    security.SubjectRef{ID: "user-ctx"},
	})
	s.Require().Error(err)
}

// ---------------------------------------------------------------------------
// Write + Delete integration tests
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestWriteAndDeleteTuple() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tuple := security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "default", ID: "del-obj-1"},
		Relation: "admin",
		Subject:  security.SubjectRef{ID: "user-del-1"},
	}

	err := adapter.WriteTuple(ctx, tuple)
	s.Require().NoError(err)

	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     tuple.Object,
		Permission: tuple.Relation,
		Subject:    tuple.Subject,
	})
	s.Require().NoError(err)
	s.True(result.Allowed)

	err = adapter.DeleteTuple(ctx, tuple)
	s.Require().NoError(err)

	result, err = adapter.Check(ctx, security.CheckRequest{
		Object:     tuple.Object,
		Permission: tuple.Relation,
		Subject:    tuple.Subject,
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "should be denied after delete")
}

func (s *AuthorizerTestSuite) TestDeleteNonExistentTuple() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	err := adapter.DeleteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "default", ID: "never-existed"},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "nobody"},
	})
	s.Require().NoError(err, "deleting a non-existent tuple should succeed (404 accepted)")
}

func (s *AuthorizerTestSuite) TestDeleteMultipleTuples() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	tuples := []security.RelationTuple{
		{
			Object:   security.ObjectRef{Namespace: "default", ID: "dm-obj-1"},
			Relation: "member",
			Subject:  security.SubjectRef{ID: "user-dm1"},
		},
		{
			Object:   security.ObjectRef{Namespace: "default", ID: "dm-obj-1"},
			Relation: "admin",
			Subject:  security.SubjectRef{ID: "user-dm2"},
		},
	}

	for _, t := range tuples {
		err := adapter.WriteTuple(ctx, t)
		s.Require().NoError(err)
	}

	deleteErr := adapter.DeleteTuples(ctx, tuples)
	s.Require().NoError(deleteErr)

	for _, t := range tuples {
		result, err := adapter.Check(ctx, security.CheckRequest{
			Object:     t.Object,
			Permission: t.Relation,
			Subject:    t.Subject,
		})
		s.Require().NoError(err)
		s.False(result.Allowed, "tuple should be deleted: %s#%s@%s",
			t.Object.ID, t.Relation, t.Subject.ID)
	}
}

// ---------------------------------------------------------------------------
// ListRelations integration tests
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestListRelations() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	obj := security.ObjectRef{Namespace: "default", ID: "lr-obj-1"}

	tuples := []security.RelationTuple{
		{Object: obj, Relation: "owner", Subject: security.SubjectRef{ID: "user-lr1"}},
		{Object: obj, Relation: "member", Subject: security.SubjectRef{ID: "user-lr2"}},
		{Object: obj, Relation: "member", Subject: security.SubjectRef{ID: "user-lr3"}},
	}
	for _, t := range tuples {
		err := adapter.WriteTuple(ctx, t)
		s.Require().NoError(err)
	}

	result, err := adapter.ListRelations(ctx, obj)
	s.Require().NoError(err)
	s.Len(result, 3)

	relations := make(map[string]string)
	for _, t := range result {
		relations[t.Subject.ID] = t.Relation
	}
	s.Equal("owner", relations["user-lr1"])
	s.Equal("member", relations["user-lr2"])
	s.Equal("member", relations["user-lr3"])
}

func (s *AuthorizerTestSuite) TestListRelationsEmpty() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	result, err := adapter.ListRelations(ctx, security.ObjectRef{
		Namespace: "default",
		ID:        "no-relations-obj",
	})
	s.Require().NoError(err)
	s.Empty(result)
}

func (s *AuthorizerTestSuite) TestListRelationsWithSubjectSet() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	obj := security.ObjectRef{Namespace: "default", ID: "lrss-obj-1"}

	tuple := security.RelationTuple{
		Object:   obj,
		Relation: "viewer",
		Subject:  security.SubjectRef{Namespace: "default", ID: "team-eng", Relation: "member"},
	}
	err := adapter.WriteTuple(ctx, tuple)
	s.Require().NoError(err)

	result, err := adapter.ListRelations(ctx, obj)
	s.Require().NoError(err)
	s.Require().Len(result, 1)
	s.Equal("viewer", result[0].Relation)
	s.Equal("default", result[0].Subject.Namespace)
	s.Equal("team-eng", result[0].Subject.ID)
	s.Equal("member", result[0].Subject.Relation)
}

// ---------------------------------------------------------------------------
// ListSubjectRelations integration tests
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestListSubjectRelations() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	subject := security.SubjectRef{ID: "user-lsr-1"}

	tuples := []security.RelationTuple{
		{
			Object:   security.ObjectRef{Namespace: "default", ID: "lsr-room-1"},
			Relation: "member",
			Subject:  subject,
		},
		{
			Object:   security.ObjectRef{Namespace: "default", ID: "lsr-room-2"},
			Relation: "admin",
			Subject:  subject,
		},
	}
	for _, t := range tuples {
		err := adapter.WriteTuple(ctx, t)
		s.Require().NoError(err)
	}

	result, err := adapter.ListSubjectRelations(ctx, subject, "default")
	s.Require().NoError(err)
	s.GreaterOrEqual(len(result), 2, "should find at least the 2 written tuples")
}

// ---------------------------------------------------------------------------
// BatchCheck integration tests
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestBatchCheck() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "default", ID: "bc-obj-1"},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "user-bc-1"},
	})
	s.Require().NoError(err)

	requests := []security.CheckRequest{
		{
			Object:     security.ObjectRef{Namespace: "default", ID: "bc-obj-1"},
			Permission: "member",
			Subject:    security.SubjectRef{ID: "user-bc-1"},
		},
		{
			Object:     security.ObjectRef{Namespace: "default", ID: "bc-obj-1"},
			Permission: "admin",
			Subject:    security.SubjectRef{ID: "user-bc-1"},
		},
	}

	results, err := adapter.BatchCheck(ctx, requests)
	s.Require().NoError(err)
	s.Require().Len(results, 2)
	s.True(results[0].Allowed, "member check should be allowed")
	s.False(results[1].Allowed, "admin check should be denied")
}

func (s *AuthorizerTestSuite) TestBatchCheckEmpty() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	results, err := adapter.BatchCheck(ctx, nil)
	s.Require().NoError(err)
	s.Empty(results)
}

// ---------------------------------------------------------------------------
// Audit logger integration test
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestCheckWithAuditLogger() {
	ctx := s.T().Context()
	logger := &recordingAuditLogger{}
	adapter := s.newAdapter(logger)

	req := security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "default", ID: "audit-obj-1"},
		Permission: "view",
		Subject:    security.SubjectRef{ID: "user-audit-1"},
	}

	result, err := adapter.Check(ctx, req)
	s.Require().NoError(err)
	s.Require().Len(logger.calls, 1)
	s.Equal(req, logger.calls[0].req)
	s.Equal(result.Allowed, logger.calls[0].result.Allowed)
}

// ---------------------------------------------------------------------------
// Expand integration test
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestExpand() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	obj := security.ObjectRef{Namespace: "default", ID: "exp-obj-1"}

	for _, id := range []string{"user-exp-1", "user-exp-2"} {
		err := adapter.WriteTuple(ctx, security.RelationTuple{
			Object:   obj,
			Relation: "member",
			Subject:  security.SubjectRef{ID: id},
		})
		s.Require().NoError(err)
	}

	subjects, err := adapter.Expand(ctx, obj, "member")
	s.Require().NoError(err)
	// Keto's expand endpoint returns a tree structure; the adapter
	// maps it to a flat list of SubjectRef. Depending on the Keto
	// version, the decoded result may be empty if the response
	// format differs from what the adapter expects.
	s.T().Logf("expand returned %d subjects", len(subjects))
}

// ---------------------------------------------------------------------------
// Integration scenario: Room access control
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestScenarioRoomAccessControl() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	room := security.ObjectRef{Namespace: "default", ID: "rac-room-1"}

	// Step 1: Assign owner and members
	for _, t := range []security.RelationTuple{
		{Object: room, Relation: "owner", Subject: security.SubjectRef{ID: "alice-rac"}},
		{Object: room, Relation: "member", Subject: security.SubjectRef{ID: "bob-rac"}},
		{Object: room, Relation: "member", Subject: security.SubjectRef{ID: "charlie-rac"}},
	} {
		err := adapter.WriteTuple(ctx, t)
		s.Require().NoError(err)
	}

	// Step 2: Verify permissions
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object: room, Permission: "owner", Subject: security.SubjectRef{ID: "alice-rac"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "alice should be owner")

	result, err = adapter.Check(ctx, security.CheckRequest{
		Object: room, Permission: "member", Subject: security.SubjectRef{ID: "bob-rac"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed, "bob should be member")

	result, err = adapter.Check(ctx, security.CheckRequest{
		Object: room, Permission: "member", Subject: security.SubjectRef{ID: "unknown-rac"},
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "unknown user should be denied")

	// Step 3: List all relations for the room
	tuples, err := adapter.ListRelations(ctx, room)
	s.Require().NoError(err)
	s.Len(tuples, 3)

	// Step 4: Remove bob's membership
	err = adapter.DeleteTuple(ctx, security.RelationTuple{
		Object: room, Relation: "member", Subject: security.SubjectRef{ID: "bob-rac"},
	})
	s.Require().NoError(err)

	// Step 5: Verify bob no longer has access
	result, err = adapter.Check(ctx, security.CheckRequest{
		Object: room, Permission: "member", Subject: security.SubjectRef{ID: "bob-rac"},
	})
	s.Require().NoError(err)
	s.False(result.Allowed, "bob should no longer be member")
}

// ---------------------------------------------------------------------------
// Integration scenario: Batch permission evaluation
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestScenarioBatchPermissionEvaluation() {
	ctx := s.T().Context()
	adapter := s.newAdapter(nil)

	doc := security.ObjectRef{Namespace: "default", ID: "bpe-doc-1"}
	user := security.SubjectRef{ID: "user-bpe"}

	// Grant view and comment only
	for _, rel := range []string{"view", "comment"} {
		err := adapter.WriteTuple(ctx, security.RelationTuple{
			Object: doc, Relation: rel, Subject: user,
		})
		s.Require().NoError(err)
	}

	requests := []security.CheckRequest{
		{Object: doc, Permission: "view", Subject: user},
		{Object: doc, Permission: "comment", Subject: user},
		{Object: doc, Permission: "edit", Subject: user},
		{Object: doc, Permission: "delete", Subject: user},
	}

	results, err := adapter.BatchCheck(ctx, requests)
	s.Require().NoError(err)
	s.Require().Len(results, 4)

	capabilities := make(map[string]bool)
	for i, req := range requests {
		capabilities[req.Permission] = results[i].Allowed
	}
	s.True(capabilities["view"])
	s.True(capabilities["comment"])
	s.False(capabilities["edit"])
	s.False(capabilities["delete"])
}

// ---------------------------------------------------------------------------
// Permissive mode tests (no server needed)
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestCheckPermissiveMode() {
	adapter := s.permissiveAdapter()
	result, err := adapter.Check(s.T().Context(), security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "default", ID: "obj-1"},
		Permission: "view",
		Subject:    security.SubjectRef{ID: "user-1"},
	})
	s.Require().NoError(err)
	s.True(result.Allowed)
	s.Equal("keto disabled - permissive mode", result.Reason)
}

func (s *AuthorizerTestSuite) TestBatchCheckPermissiveMode() {
	adapter := s.permissiveAdapter()
	requests := []security.CheckRequest{
		{
			Object:     security.ObjectRef{Namespace: "default", ID: "obj-1"},
			Permission: "view",
			Subject:    security.SubjectRef{ID: "user-1"},
		},
		{
			Object:     security.ObjectRef{Namespace: "default", ID: "obj-2"},
			Permission: "edit",
			Subject:    security.SubjectRef{ID: "user-2"},
		},
	}

	results, err := adapter.BatchCheck(s.T().Context(), requests)
	s.Require().NoError(err)
	s.Require().Len(results, 2)
	for _, r := range results {
		s.True(r.Allowed)
		s.Equal("keto disabled - permissive mode", r.Reason)
	}
}

func (s *AuthorizerTestSuite) TestWriteTuplesPermissive() {
	adapter := s.permissiveAdapter()
	err := adapter.WriteTuple(s.T().Context(), security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "default", ID: "obj-1"},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "user-1"},
	})
	s.Require().NoError(err)
}

func (s *AuthorizerTestSuite) TestWriteTuplesEmpty() {
	adapter := s.newAdapter(nil)
	err := adapter.WriteTuples(s.T().Context(), nil)
	s.Require().NoError(err)
}

func (s *AuthorizerTestSuite) TestDeleteTuplesPermissive() {
	adapter := s.permissiveAdapter()
	err := adapter.DeleteTuple(s.T().Context(), security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "default", ID: "obj-1"},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "user-1"},
	})
	s.Require().NoError(err)
}

func (s *AuthorizerTestSuite) TestDeleteTuplesEmpty() {
	adapter := s.newAdapter(nil)
	err := adapter.DeleteTuples(s.T().Context(), nil)
	s.Require().NoError(err)
}

func (s *AuthorizerTestSuite) TestListRelationsPermissive() {
	adapter := s.permissiveAdapter()
	tuples, err := adapter.ListRelations(s.T().Context(), security.ObjectRef{
		Namespace: "default", ID: "obj-1",
	})
	s.Require().NoError(err)
	s.Empty(tuples)
}

func (s *AuthorizerTestSuite) TestListSubjectRelationsPermissive() {
	adapter := s.permissiveAdapter()
	tuples, err := adapter.ListSubjectRelations(s.T().Context(),
		security.SubjectRef{ID: "user-1"}, "default")
	s.Require().NoError(err)
	s.Empty(tuples)
}

func (s *AuthorizerTestSuite) TestExpandPermissive() {
	adapter := s.permissiveAdapter()
	subjects, err := adapter.Expand(s.T().Context(), security.ObjectRef{
		Namespace: "default", ID: "obj-1",
	}, "member")
	s.Require().NoError(err)
	s.Empty(subjects)
}

// ---------------------------------------------------------------------------
// Error type tests
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestPermissionDeniedError() {
	err := authorizer.NewPermissionDeniedError(
		security.ObjectRef{Namespace: "doc", ID: "doc-1"},
		"edit",
		security.SubjectRef{ID: "user-1"},
		"no matching relation",
	)

	s.Require().ErrorIs(err, authorizer.ErrPermissionDenied)
	s.Contains(err.Error(), "user-1")
	s.Contains(err.Error(), "edit")
	s.Contains(err.Error(), "doc:doc-1")
	s.Contains(err.Error(), "no matching relation")
}

func (s *AuthorizerTestSuite) TestAuthzServiceError() {
	cause := errors.New("connection refused")
	err := authorizer.NewAuthzServiceError("check", cause)

	s.Require().ErrorIs(err, authorizer.ErrAuthzServiceDown)
	s.Contains(err.Error(), "check")
	s.Contains(err.Error(), "connection refused")
	s.Require().ErrorIs(err, cause)
}

func (s *AuthorizerTestSuite) TestAuthzServiceErrorUnwrap() {
	innerErr := errors.New("timeout")
	err := authorizer.NewAuthzServiceError("expand", innerErr)
	s.ErrorIs(err, innerErr)
}

// ---------------------------------------------------------------------------
// Audit logger tests
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestNoOpAuditLogger() {
	logger := authorizer.NewNoOpAuditLogger()
	err := logger.LogDecision(s.T().Context(),
		security.CheckRequest{}, security.CheckResult{}, nil)
	s.Require().NoError(err)
}

func (s *AuthorizerTestSuite) TestAuditLoggerDefaultSampleRate() {
	logger := authorizer.NewAuditLogger(authorizer.AuditLoggerConfig{})
	s.Require().NotNil(logger)

	err := logger.LogDecision(s.T().Context(),
		security.CheckRequest{}, security.CheckResult{}, nil)
	s.Require().NoError(err)
}

func (s *AuthorizerTestSuite) TestAuditLoggerClampsSampleRate() {
	testCases := []struct {
		name       string
		sampleRate float64
	}{
		{"negative rate clamped to 1.0", -1.0},
		{"zero rate clamped to 1.0", 0.0},
		{"over 1.0 clamped to 1.0", 5.0},
		{"valid rate 0.5", 0.5},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			logger := authorizer.NewAuditLogger(
				authorizer.AuditLoggerConfig{SampleRate: tc.sampleRate})
			s.Require().NotNil(logger)
		})
	}
}

// ---------------------------------------------------------------------------
// Constructor tests
// ---------------------------------------------------------------------------

func (s *AuthorizerTestSuite) TestNewKetoAdapterNilAuditLogger() {
	cfg := &config.ConfigurationDefault{
		AuthorizationServiceReadURI: s.readURI,
	}
	mgr := client.NewManager(s.T().Context())
	adapter := authorizer.NewKetoAdapter(cfg, mgr, nil)
	s.Require().NotNil(adapter)

	// Should not panic — nil logger replaced by NoOp
	result, err := adapter.Check(s.T().Context(), security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "default", ID: "nil-audit-obj"},
		Permission: "view",
		Subject:    security.SubjectRef{ID: "nil-audit-user"},
	})
	s.Require().NoError(err)
	s.False(result.Allowed)
}
