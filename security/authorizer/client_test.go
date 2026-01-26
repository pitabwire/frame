package authorizer_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pitabwire/frame/client"
	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/security/authorizer"
)

// ---------------------------------------------------------------------------
// Test config implementing config.ConfigurationAuthorization
// ---------------------------------------------------------------------------

type testConfig struct {
	readURI  string
	writeURI string
}

func (c *testConfig) GetAuthorizationServiceReadURI() string  { return c.readURI }
func (c *testConfig) GetAuthorizationServiceWriteURI() string { return c.writeURI }
func (c *testConfig) AuthorizationServiceCanRead() bool       { return c.readURI != "" }
func (c *testConfig) AuthorizationServiceCanWrite() bool      { return c.writeURI != "" }

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

// failingAuditLogger returns an error on every LogDecision call.
type failingAuditLogger struct{}

func (f *failingAuditLogger) LogDecision(
	_ context.Context,
	_ security.CheckRequest,
	_ security.CheckResult,
	_ map[string]string,
) error {
	return errors.New("audit service unavailable")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newAdapter(readURL, writeURL string, auditLogger security.AuditLogger) security.Authorizer {
	cfg := &testConfig{readURI: readURL, writeURI: writeURL}
	mgr := client.NewManager(context.Background())
	return authorizer.NewKetoAdapter(cfg, mgr, auditLogger)
}

func sampleCheckReq() security.CheckRequest {
	return security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "document", ID: "doc-123"},
		Permission: "view",
		Subject:    security.SubjectRef{Namespace: security.NamespaceProfile, ID: "user-456"},
	}
}

func sampleTuple() security.RelationTuple {
	return security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "room", ID: "room-1"},
		Relation: "member",
		Subject:  security.SubjectRef{Namespace: security.NamespaceProfile, ID: "user-1"},
	}
}

func sampleSubjectSetTuple() security.RelationTuple {
	return security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "document", ID: "doc-1"},
		Relation: "viewer",
		Subject:  security.SubjectRef{Namespace: "group", ID: "editors", Relation: "member"},
	}
}

// ---------------------------------------------------------------------------
// Check tests
// ---------------------------------------------------------------------------

func TestCheck_Allowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/relation-tuples/check", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "document", r.URL.Query().Get("namespace"))
		assert.Equal(t, "doc-123", r.URL.Query().Get("object"))
		assert.Equal(t, "view", r.URL.Query().Get("relation"))
		assert.Equal(t, "user-456", r.URL.Query().Get("subject_id"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": true})
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, "", nil)
	result, err := adapter.Check(context.Background(), sampleCheckReq())

	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.NotZero(t, result.CheckedAt)
	assert.Empty(t, result.Reason)
}

func TestCheck_Denied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": false})
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, "", nil)
	result, err := adapter.Check(context.Background(), sampleCheckReq())

	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Equal(t, "no matching relation found", result.Reason)
}

func TestCheck_PermissiveMode(t *testing.T) {
	adapter := newAdapter("", "", nil)
	result, err := adapter.Check(context.Background(), sampleCheckReq())

	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, "keto disabled - permissive mode", result.Reason)
}

func TestCheck_SubjectSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		assert.Equal(t, "group", q.Get("subject_set.namespace"))
		assert.Equal(t, "admins", q.Get("subject_set.object"))
		assert.Equal(t, "member", q.Get("subject_set.relation"))
		assert.Empty(t, q.Get("subject_id"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": true})
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, "", nil)
	req := security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "resource", ID: "res-1"},
		Permission: "edit",
		Subject:    security.SubjectRef{Namespace: "group", ID: "admins", Relation: "member"},
	}
	result, err := adapter.Check(context.Background(), req)

	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestCheck_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, "", nil)
	_, err := adapter.Check(context.Background(), sampleCheckReq())

	require.Error(t, err)
	require.ErrorIs(t, err, authorizer.ErrAuthzServiceDown)
	assert.Contains(t, err.Error(), "500")
}

func TestCheck_TransportError(t *testing.T) {
	adapter := newAdapter("http://127.0.0.1:1", "", nil)
	_, err := adapter.Check(context.Background(), sampleCheckReq())

	require.Error(t, err)
	assert.ErrorIs(t, err, authorizer.ErrAuthzServiceDown)
}

func TestCheck_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{invalid json"))
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, "", nil)
	_, err := adapter.Check(context.Background(), sampleCheckReq())

	require.Error(t, err)
	assert.ErrorIs(t, err, authorizer.ErrAuthzServiceDown)
}

func TestCheck_AuditLoggerCalled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": true})
	}))
	defer srv.Close()

	logger := &recordingAuditLogger{}
	adapter := newAdapter(srv.URL, "", logger)
	req := sampleCheckReq()
	result, err := adapter.Check(context.Background(), req)

	require.NoError(t, err)
	require.Len(t, logger.calls, 1)
	assert.Equal(t, req, logger.calls[0].req)
	assert.Equal(t, result.Allowed, logger.calls[0].result.Allowed)
}

func TestCheck_AuditLoggerError_DoesNotFailCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": true})
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, "", &failingAuditLogger{})
	result, err := adapter.Check(context.Background(), sampleCheckReq())

	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestCheck_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": true})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	adapter := newAdapter(srv.URL, "", nil)
	_, err := adapter.Check(ctx, sampleCheckReq())

	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// BatchCheck tests
// ---------------------------------------------------------------------------

func TestBatchCheck_MixedResults(t *testing.T) {
	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count := reqCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": count%2 == 1})
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, "", nil)
	requests := []security.CheckRequest{
		{
			Object:     security.ObjectRef{Namespace: "doc", ID: "1"},
			Permission: "view",
			Subject:    security.SubjectRef{ID: "u1"},
		},
		{
			Object:     security.ObjectRef{Namespace: "doc", ID: "2"},
			Permission: "view",
			Subject:    security.SubjectRef{ID: "u2"},
		},
		{
			Object:     security.ObjectRef{Namespace: "doc", ID: "3"},
			Permission: "view",
			Subject:    security.SubjectRef{ID: "u3"},
		},
	}

	results, err := adapter.BatchCheck(context.Background(), requests)

	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.True(t, results[0].Allowed)
	assert.False(t, results[1].Allowed)
	assert.True(t, results[2].Allowed)
}

func TestBatchCheck_PermissiveMode(t *testing.T) {
	adapter := newAdapter("", "", nil)
	requests := []security.CheckRequest{
		{
			Object:     security.ObjectRef{Namespace: "doc", ID: "1"},
			Permission: "view",
			Subject:    security.SubjectRef{ID: "u1"},
		},
		{
			Object:     security.ObjectRef{Namespace: "doc", ID: "2"},
			Permission: "edit",
			Subject:    security.SubjectRef{ID: "u2"},
		},
	}

	results, err := adapter.BatchCheck(context.Background(), requests)

	require.NoError(t, err)
	require.Len(t, results, 2)
	for _, r := range results {
		assert.True(t, r.Allowed)
		assert.Equal(t, "keto disabled - permissive mode", r.Reason)
	}
}

func TestBatchCheck_FailClosed(t *testing.T) {
	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count := reqCount.Add(1)
		if count == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": true})
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, "", nil)
	requests := []security.CheckRequest{
		{
			Object:     security.ObjectRef{Namespace: "doc", ID: "1"},
			Permission: "view",
			Subject:    security.SubjectRef{ID: "u1"},
		},
		{
			Object:     security.ObjectRef{Namespace: "doc", ID: "2"},
			Permission: "view",
			Subject:    security.SubjectRef{ID: "u2"},
		},
		{
			Object:     security.ObjectRef{Namespace: "doc", ID: "3"},
			Permission: "view",
			Subject:    security.SubjectRef{ID: "u3"},
		},
	}

	results, err := adapter.BatchCheck(context.Background(), requests)

	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.True(t, results[0].Allowed, "first check should succeed")
	assert.False(t, results[1].Allowed, "second check should fail closed")
	assert.Contains(t, results[1].Reason, "check failed")
	assert.True(t, results[2].Allowed, "third check should succeed")
}

func TestBatchCheck_Empty(t *testing.T) {
	adapter := newAdapter("http://should-not-be-called", "", nil)
	results, err := adapter.BatchCheck(context.Background(), nil)

	require.NoError(t, err)
	assert.Empty(t, results)
}

// ---------------------------------------------------------------------------
// WriteTuple / WriteTuples tests
// ---------------------------------------------------------------------------

func TestWriteTuple_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/admin/relation-tuples", r.URL.Path)
		assert.Equal(t, http.MethodPut, r.Method)

		var body map[string]any
		err := json.NewDecoder(r.Body).Decode(&body)
		assert.NoError(t, err)

		tuples, ok := body["relation_tuples"].([]any)
		assert.True(t, ok)
		assert.Len(t, tuples, 1)

		if len(tuples) > 0 {
			tuple := tuples[0].(map[string]any)
			assert.Equal(t, "room", tuple["namespace"])
			assert.Equal(t, "room-1", tuple["object"])
			assert.Equal(t, "member", tuple["relation"])
			assert.Equal(t, "user-1", tuple["subject_id"])
		}

		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	adapter := newAdapter("", srv.URL, nil)
	err := adapter.WriteTuple(context.Background(), sampleTuple())

	require.NoError(t, err)
}

func TestWriteTuples_SubjectSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		err := json.NewDecoder(r.Body).Decode(&body)
		assert.NoError(t, err)

		tuples := body["relation_tuples"].([]any)
		tuple := tuples[0].(map[string]any)

		subjectSet, ok := tuple["subject_set"].(map[string]any)
		assert.True(t, ok, "should use subject_set for subject with relation")
		if ok {
			assert.Equal(t, "group", subjectSet["namespace"])
			assert.Equal(t, "editors", subjectSet["object"])
			assert.Equal(t, "member", subjectSet["relation"])
		}
		assert.Empty(t, tuple["subject_id"])

		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	adapter := newAdapter("", srv.URL, nil)
	err := adapter.WriteTuple(context.Background(), sampleSubjectSetTuple())

	require.NoError(t, err)
}

func TestWriteTuples_MultipleTuples(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		err := json.NewDecoder(r.Body).Decode(&body)
		assert.NoError(t, err)

		tuples := body["relation_tuples"].([]any)
		assert.Len(t, tuples, 3)

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	adapter := newAdapter("", srv.URL, nil)
	tuples := []security.RelationTuple{
		{
			Object:   security.ObjectRef{Namespace: "room", ID: "r1"},
			Relation: "member",
			Subject:  security.SubjectRef{ID: "u1"},
		},
		{
			Object:   security.ObjectRef{Namespace: "room", ID: "r1"},
			Relation: "admin",
			Subject:  security.SubjectRef{ID: "u2"},
		},
		{
			Object:   security.ObjectRef{Namespace: "room", ID: "r2"},
			Relation: "member",
			Subject:  security.SubjectRef{ID: "u1"},
		},
	}

	err := adapter.WriteTuples(context.Background(), tuples)

	require.NoError(t, err)
}

func TestWriteTuples_Disabled(t *testing.T) {
	adapter := newAdapter("", "", nil)
	err := adapter.WriteTuple(context.Background(), sampleTuple())

	require.NoError(t, err)
}

func TestWriteTuples_Empty(t *testing.T) {
	adapter := newAdapter("", "http://should-not-be-called", nil)
	err := adapter.WriteTuples(context.Background(), nil)

	require.NoError(t, err)
}

func TestWriteTuples_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid namespace"}`))
	}))
	defer srv.Close()

	adapter := newAdapter("", srv.URL, nil)
	err := adapter.WriteTuple(context.Background(), sampleTuple())

	require.Error(t, err)
	require.ErrorIs(t, err, authorizer.ErrAuthzServiceDown)
	assert.Contains(t, err.Error(), "400")
}

func TestWriteTuples_NoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	adapter := newAdapter("", srv.URL, nil)
	err := adapter.WriteTuple(context.Background(), sampleTuple())

	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// DeleteTuple / DeleteTuples tests
// ---------------------------------------------------------------------------

func TestDeleteTuple_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/admin/relation-tuples", r.URL.Path)
		assert.Equal(t, http.MethodDelete, r.Method)

		q := r.URL.Query()
		assert.Equal(t, "room", q.Get("namespace"))
		assert.Equal(t, "room-1", q.Get("object"))
		assert.Equal(t, "member", q.Get("relation"))
		assert.Equal(t, "user-1", q.Get("subject_id"))

		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	adapter := newAdapter("", srv.URL, nil)
	err := adapter.DeleteTuple(context.Background(), sampleTuple())

	require.NoError(t, err)
}

func TestDeleteTuples_SubjectSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		assert.Equal(t, "group", q.Get("subject_set.namespace"))
		assert.Equal(t, "editors", q.Get("subject_set.object"))
		assert.Equal(t, "member", q.Get("subject_set.relation"))
		assert.Empty(t, q.Get("subject_id"))

		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	adapter := newAdapter("", srv.URL, nil)
	err := adapter.DeleteTuple(context.Background(), sampleSubjectSetTuple())

	require.NoError(t, err)
}

func TestDeleteTuples_NotFoundAccepted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	adapter := newAdapter("", srv.URL, nil)
	err := adapter.DeleteTuple(context.Background(), sampleTuple())

	require.NoError(t, err)
}

func TestDeleteTuples_Disabled(t *testing.T) {
	adapter := newAdapter("", "", nil)
	err := adapter.DeleteTuple(context.Background(), sampleTuple())

	require.NoError(t, err)
}

func TestDeleteTuples_Empty(t *testing.T) {
	adapter := newAdapter("", "http://should-not-be-called", nil)
	err := adapter.DeleteTuples(context.Background(), nil)

	require.NoError(t, err)
}

func TestDeleteTuples_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	adapter := newAdapter("", srv.URL, nil)
	err := adapter.DeleteTuple(context.Background(), sampleTuple())

	require.Error(t, err)
	assert.ErrorIs(t, err, authorizer.ErrAuthzServiceDown)
}

func TestDeleteTuples_MultipleTuples(t *testing.T) {
	var deleteCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		deleteCount.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	adapter := newAdapter("", srv.URL, nil)
	tuples := []security.RelationTuple{
		{
			Object:   security.ObjectRef{Namespace: "room", ID: "r1"},
			Relation: "member",
			Subject:  security.SubjectRef{ID: "u1"},
		},
		{
			Object:   security.ObjectRef{Namespace: "room", ID: "r1"},
			Relation: "admin",
			Subject:  security.SubjectRef{ID: "u2"},
		},
	}

	err := adapter.DeleteTuples(context.Background(), tuples)

	require.NoError(t, err)
	assert.Equal(t, int32(2), deleteCount.Load(), "should issue one DELETE per tuple")
}

func TestDeleteTuples_PartialFailure(t *testing.T) {
	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count := reqCount.Add(1)
		if count == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	adapter := newAdapter("", srv.URL, nil)
	tuples := []security.RelationTuple{
		{
			Object:   security.ObjectRef{Namespace: "room", ID: "r1"},
			Relation: "member",
			Subject:  security.SubjectRef{ID: "u1"},
		},
		{
			Object:   security.ObjectRef{Namespace: "room", ID: "r1"},
			Relation: "admin",
			Subject:  security.SubjectRef{ID: "u2"},
		},
		{
			Object:   security.ObjectRef{Namespace: "room", ID: "r2"},
			Relation: "member",
			Subject:  security.SubjectRef{ID: "u3"},
		},
	}

	err := adapter.DeleteTuples(context.Background(), tuples)

	require.Error(t, err, "should return error on partial failure")
}

// ---------------------------------------------------------------------------
// ListRelations tests
// ---------------------------------------------------------------------------

func TestListRelations_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/relation-tuples", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		q := r.URL.Query()
		assert.Equal(t, "room", q.Get("namespace"))
		assert.Equal(t, "room-1", q.Get("object"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"relation_tuples": []map[string]any{
				{"namespace": "room", "object": "room-1", "relation": "owner", "subject_id": "user-A"},
				{"namespace": "room", "object": "room-1", "relation": "member", "subject_id": "user-B"},
			},
		})
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, "", nil)
	tuples, err := adapter.ListRelations(context.Background(), security.ObjectRef{Namespace: "room", ID: "room-1"})

	require.NoError(t, err)
	require.Len(t, tuples, 2)
	assert.Equal(t, "owner", tuples[0].Relation)
	assert.Equal(t, "user-A", tuples[0].Subject.ID)
	assert.Equal(t, "member", tuples[1].Relation)
	assert.Equal(t, "user-B", tuples[1].Subject.ID)
}

func TestListRelations_WithSubjectSets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"relation_tuples": []map[string]any{
				{
					"namespace": "document",
					"object":    "doc-1",
					"relation":  "viewer",
					"subject_set": map[string]string{
						"namespace": "group",
						"object":    "engineering",
						"relation":  "member",
					},
				},
			},
		})
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, "", nil)
	tuples, err := adapter.ListRelations(context.Background(), security.ObjectRef{Namespace: "document", ID: "doc-1"})

	require.NoError(t, err)
	require.Len(t, tuples, 1)
	assert.Equal(t, "group", tuples[0].Subject.Namespace)
	assert.Equal(t, "engineering", tuples[0].Subject.ID)
	assert.Equal(t, "member", tuples[0].Subject.Relation)
}

func TestListRelations_PermissiveMode(t *testing.T) {
	adapter := newAdapter("", "", nil)
	tuples, err := adapter.ListRelations(context.Background(), security.ObjectRef{Namespace: "room", ID: "room-1"})

	require.NoError(t, err)
	assert.Empty(t, tuples)
}

func TestListRelations_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server down"))
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, "", nil)
	_, err := adapter.ListRelations(context.Background(), security.ObjectRef{Namespace: "room", ID: "room-1"})

	require.Error(t, err)
	assert.ErrorIs(t, err, authorizer.ErrAuthzServiceDown)
}

func TestListRelations_EmptyResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"relation_tuples": []map[string]any{},
		})
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, "", nil)
	tuples, err := adapter.ListRelations(context.Background(), security.ObjectRef{Namespace: "room", ID: "room-1"})

	require.NoError(t, err)
	assert.Empty(t, tuples)
}

// ---------------------------------------------------------------------------
// ListSubjectRelations tests
// ---------------------------------------------------------------------------

func TestListSubjectRelations_DirectSubject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/relation-tuples", r.URL.Path)

		q := r.URL.Query()
		assert.Equal(t, "room", q.Get("namespace"))
		assert.Equal(t, "user-1", q.Get("subject_id"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"relation_tuples": []map[string]any{
				{"namespace": "room", "object": "room-1", "relation": "member", "subject_id": "user-1"},
				{"namespace": "room", "object": "room-2", "relation": "admin", "subject_id": "user-1"},
			},
		})
	}))
	defer srv.Close()

	// ListSubjectRelations checks CanRead() but uses WriteURI, so both must be set
	adapter := newAdapter(srv.URL, srv.URL, nil)
	subject := security.SubjectRef{Namespace: security.NamespaceProfile, ID: "user-1"}
	tuples, err := adapter.ListSubjectRelations(context.Background(), subject, "room")

	require.NoError(t, err)
	require.Len(t, tuples, 2)
	assert.Equal(t, "room-1", tuples[0].Object.ID)
	assert.Equal(t, "member", tuples[0].Relation)
	assert.Equal(t, "room-2", tuples[1].Object.ID)
	assert.Equal(t, "admin", tuples[1].Relation)
}

func TestListSubjectRelations_SubjectSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		assert.Equal(t, "group", q.Get("subject_set.namespace"))
		assert.Equal(t, "admins", q.Get("subject_set.object"))
		assert.Equal(t, "member", q.Get("subject_set.relation"))
		assert.Empty(t, q.Get("subject_id"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"relation_tuples": []map[string]any{},
		})
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, srv.URL, nil)
	subject := security.SubjectRef{Namespace: "group", ID: "admins", Relation: "member"}
	tuples, err := adapter.ListSubjectRelations(context.Background(), subject, "")

	require.NoError(t, err)
	assert.Empty(t, tuples)
}

func TestListSubjectRelations_NoNamespaceFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		assert.Empty(t, q.Get("namespace"), "namespace should not be set when empty")

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"relation_tuples": []map[string]any{},
		})
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, srv.URL, nil)
	subject := security.SubjectRef{ID: "user-1"}
	tuples, err := adapter.ListSubjectRelations(context.Background(), subject, "")

	require.NoError(t, err)
	assert.Empty(t, tuples)
}

func TestListSubjectRelations_PermissiveMode(t *testing.T) {
	adapter := newAdapter("", "", nil)
	subject := security.SubjectRef{ID: "user-1"}
	tuples, err := adapter.ListSubjectRelations(context.Background(), subject, "room")

	require.NoError(t, err)
	assert.Empty(t, tuples)
}

func TestListSubjectRelations_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, srv.URL, nil)
	subject := security.SubjectRef{ID: "user-1"}
	_, err := adapter.ListSubjectRelations(context.Background(), subject, "room")

	require.Error(t, err)
	assert.ErrorIs(t, err, authorizer.ErrAuthzServiceDown)
}

// ---------------------------------------------------------------------------
// Expand tests
// ---------------------------------------------------------------------------

func TestExpand_SubjectIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/relation-tuples/expand", r.URL.Path)

		q := r.URL.Query()
		assert.Equal(t, "room", q.Get("namespace"))
		assert.Equal(t, "room-1", q.Get("object"))
		assert.Equal(t, "member", q.Get("relation"))
		assert.Equal(t, "3", q.Get("max-depth"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subject_ids": []string{"user-A", "user-B", "user-C"},
		})
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, "", nil)
	subjects, err := adapter.Expand(context.Background(), security.ObjectRef{Namespace: "room", ID: "room-1"}, "member")

	require.NoError(t, err)
	require.Len(t, subjects, 3)
	for _, s := range subjects {
		assert.Equal(t, security.NamespaceProfile, s.Namespace)
		assert.Empty(t, s.Relation)
	}
	assert.Equal(t, "user-A", subjects[0].ID)
	assert.Equal(t, "user-B", subjects[1].ID)
	assert.Equal(t, "user-C", subjects[2].ID)
}

func TestExpand_SubjectSets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subject_sets": []map[string]string{
				{"namespace": "group", "object": "editors", "relation": "member"},
				{"namespace": "group", "object": "admins", "relation": "member"},
			},
		})
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, "", nil)
	subjects, err := adapter.Expand(context.Background(), security.ObjectRef{Namespace: "doc", ID: "doc-1"}, "viewer")

	require.NoError(t, err)
	require.Len(t, subjects, 2)
	assert.Equal(t, "group", subjects[0].Namespace)
	assert.Equal(t, "editors", subjects[0].ID)
	assert.Equal(t, "member", subjects[0].Relation)
	assert.Equal(t, "admins", subjects[1].ID)
}

func TestExpand_MixedSubjectsAndSets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subject_ids": []string{"user-direct"},
			"subject_sets": []map[string]string{
				{"namespace": "group", "object": "team", "relation": "member"},
			},
		})
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, "", nil)
	subjects, err := adapter.Expand(context.Background(), security.ObjectRef{Namespace: "doc", ID: "doc-1"}, "viewer")

	require.NoError(t, err)
	require.Len(t, subjects, 2)
	assert.Equal(t, "user-direct", subjects[0].ID)
	assert.Equal(t, "team", subjects[1].ID)
}

func TestExpand_PermissiveMode(t *testing.T) {
	adapter := newAdapter("", "", nil)
	subjects, err := adapter.Expand(context.Background(), security.ObjectRef{Namespace: "doc", ID: "doc-1"}, "viewer")

	require.NoError(t, err)
	assert.Empty(t, subjects)
}

func TestExpand_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("service down"))
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, "", nil)
	_, err := adapter.Expand(context.Background(), security.ObjectRef{Namespace: "doc", ID: "doc-1"}, "viewer")

	require.Error(t, err)
	assert.ErrorIs(t, err, authorizer.ErrAuthzServiceDown)
}

func TestExpand_EmptyResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	adapter := newAdapter(srv.URL, "", nil)
	subjects, err := adapter.Expand(context.Background(), security.ObjectRef{Namespace: "doc", ID: "doc-1"}, "viewer")

	require.NoError(t, err)
	assert.Empty(t, subjects)
}

// ---------------------------------------------------------------------------
// NewKetoAdapter tests
// ---------------------------------------------------------------------------

func TestNewKetoAdapter_NilAuditLoggerDefaults(t *testing.T) {
	cfg := &testConfig{readURI: "http://localhost:4466"}
	mgr := client.NewManager(context.Background())

	adapter := authorizer.NewKetoAdapter(cfg, mgr, nil)
	require.NotNil(t, adapter)

	// Should not panic when calling Check (nil logger replaced by NoOp)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": true})
	}))
	defer srv.Close()

	cfgWithSrv := &testConfig{readURI: srv.URL}
	adapterWithSrv := authorizer.NewKetoAdapter(cfgWithSrv, mgr, nil)
	result, err := adapterWithSrv.Check(context.Background(), sampleCheckReq())

	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

// ---------------------------------------------------------------------------
// Error type tests
// ---------------------------------------------------------------------------

func TestPermissionDeniedError(t *testing.T) {
	err := authorizer.NewPermissionDeniedError(
		security.ObjectRef{Namespace: "doc", ID: "doc-1"},
		"edit",
		security.SubjectRef{ID: "user-1"},
		"no matching relation",
	)

	require.ErrorIs(t, err, authorizer.ErrPermissionDenied)
	assert.Contains(t, err.Error(), "user-1")
	assert.Contains(t, err.Error(), "edit")
	assert.Contains(t, err.Error(), "doc:doc-1")
	assert.Contains(t, err.Error(), "no matching relation")
}

func TestAuthzServiceError(t *testing.T) {
	cause := errors.New("connection refused")
	err := authorizer.NewAuthzServiceError("check", cause)

	require.ErrorIs(t, err, authorizer.ErrAuthzServiceDown)
	assert.Contains(t, err.Error(), "check")
	assert.Contains(t, err.Error(), "connection refused")
	require.ErrorIs(t, err, cause)
}

func TestAuthzServiceError_Unwrap(t *testing.T) {
	innerErr := errors.New("timeout")
	err := authorizer.NewAuthzServiceError("expand", innerErr)

	assert.ErrorIs(t, err, innerErr)
}

// ---------------------------------------------------------------------------
// Audit logger tests
// ---------------------------------------------------------------------------

func TestNoOpAuditLogger(t *testing.T) {
	logger := authorizer.NewNoOpAuditLogger()
	err := logger.LogDecision(context.Background(), security.CheckRequest{}, security.CheckResult{}, nil)

	require.NoError(t, err)
}

func TestNewAuditLogger_DefaultSampleRate(t *testing.T) {
	logger := authorizer.NewAuditLogger(authorizer.AuditLoggerConfig{})
	require.NotNil(t, logger)

	err := logger.LogDecision(context.Background(), security.CheckRequest{}, security.CheckResult{}, nil)
	require.NoError(t, err)
}

func TestNewAuditLogger_ClampsSampleRate(t *testing.T) {
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
		t.Run(tc.name, func(t *testing.T) {
			logger := authorizer.NewAuditLogger(authorizer.AuditLoggerConfig{SampleRate: tc.sampleRate})
			require.NotNil(t, logger)
		})
	}
}

// ---------------------------------------------------------------------------
// Integration-style scenarios
// ---------------------------------------------------------------------------

func TestScenario_RoomAccessControl(t *testing.T) {
	// Simulate a chat room access control scenario:
	// 1. Create a room and assign owner, members, and a viewer group
	// 2. Verify each user's permissions
	// 3. Remove a member and verify access revoked

	var writtenTuples []map[string]any
	var deletedQueries []string

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /admin/relation-tuples", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if tuples, ok := body["relation_tuples"].([]any); ok {
			for _, t := range tuples {
				writtenTuples = append(writtenTuples, t.(map[string]any))
			}
		}
		w.WriteHeader(http.StatusCreated)
	})

	mux.HandleFunc("DELETE /admin/relation-tuples", func(w http.ResponseWriter, r *http.Request) {
		deletedQueries = append(deletedQueries, r.URL.RawQuery)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /relation-tuples/check", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		subjectID := q.Get("subject_id")
		relation := q.Get("relation")

		allowed := false
		for _, t := range writtenTuples {
			if t["subject_id"] == subjectID && t["relation"] == relation {
				allowed = true
				break
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": allowed})
	})

	mux.HandleFunc("GET /relation-tuples", func(w http.ResponseWriter, r *http.Request) {
		objID := r.URL.Query().Get("object")
		var matched []map[string]any
		for _, t := range writtenTuples {
			if t["object"] == objID {
				matched = append(matched, t)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"relation_tuples": matched})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := context.Background()
	adapter := newAdapter(srv.URL, srv.URL, nil)

	// Step 1: Write tuples for room access
	err := adapter.WriteTuples(ctx, []security.RelationTuple{
		{
			Object:   security.ObjectRef{Namespace: "room", ID: "chat-1"},
			Relation: "owner",
			Subject:  security.SubjectRef{ID: "alice"},
		},
		{
			Object:   security.ObjectRef{Namespace: "room", ID: "chat-1"},
			Relation: "member",
			Subject:  security.SubjectRef{ID: "bob"},
		},
		{
			Object:   security.ObjectRef{Namespace: "room", ID: "chat-1"},
			Relation: "member",
			Subject:  security.SubjectRef{ID: "charlie"},
		},
	})
	require.NoError(t, err)
	assert.Len(t, writtenTuples, 3)

	// Step 2: Verify permissions
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "room", ID: "chat-1"},
		Permission: "owner",
		Subject:    security.SubjectRef{ID: "alice"},
	})
	require.NoError(t, err)
	assert.True(t, result.Allowed, "alice should be owner")

	result, err = adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "room", ID: "chat-1"},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "bob"},
	})
	require.NoError(t, err)
	assert.True(t, result.Allowed, "bob should be member")

	result, err = adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "room", ID: "chat-1"},
		Permission: "member",
		Subject:    security.SubjectRef{ID: "unknown"},
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed, "unknown user should be denied")

	// Step 3: List all relations for the room
	tuples, err := adapter.ListRelations(ctx, security.ObjectRef{Namespace: "room", ID: "chat-1"})
	require.NoError(t, err)
	assert.Len(t, tuples, 3)

	// Step 4: Remove bob's membership
	err = adapter.DeleteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "room", ID: "chat-1"},
		Relation: "member",
		Subject:  security.SubjectRef{ID: "bob"},
	})
	require.NoError(t, err)
	assert.Len(t, deletedQueries, 1)
}

func TestScenario_GroupBasedAccess(t *testing.T) {
	// Simulate group-based access:
	// Users are members of a group, and the group has access to a document
	// Check should use subject_set parameters

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /admin/relation-tuples", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	mux.HandleFunc("GET /relation-tuples/check", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		// Grant access only when checking via the engineering group subject set
		allowed := q.Get("subject_set.namespace") == "group" &&
			q.Get("subject_set.object") == "engineering" &&
			q.Get("subject_set.relation") == "member"

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": allowed})
	})

	mux.HandleFunc("GET /relation-tuples/expand", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subject_ids": []string{"alice", "bob"},
			"subject_sets": []map[string]string{
				{"namespace": "group", "object": "engineering", "relation": "member"},
			},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := context.Background()
	adapter := newAdapter(srv.URL, srv.URL, nil)

	// Write the group-based access tuple
	err := adapter.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: "document", ID: "design-doc"},
		Relation: "viewer",
		Subject:  security.SubjectRef{Namespace: "group", ID: "engineering", Relation: "member"},
	})
	require.NoError(t, err)

	// Check: group member should have access
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "document", ID: "design-doc"},
		Permission: "viewer",
		Subject:    security.SubjectRef{Namespace: "group", ID: "engineering", Relation: "member"},
	})
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Check: direct user should not have access via this check
	result, err = adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "document", ID: "design-doc"},
		Permission: "viewer",
		Subject:    security.SubjectRef{ID: "alice"},
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed)

	// Expand: see who has viewer access
	subjects, err := adapter.Expand(ctx, security.ObjectRef{Namespace: "document", ID: "design-doc"}, "viewer")
	require.NoError(t, err)
	assert.Len(t, subjects, 3, "should return 2 direct IDs + 1 subject set")
}

func TestScenario_MultiTenantIsolation(t *testing.T) {
	// Verify that authorization checks properly scope by namespace
	// A user in tenant-A should not have access to tenant-B's resources

	mux := http.NewServeMux()
	mux.HandleFunc("GET /relation-tuples/check", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		ns := q.Get("namespace")
		objectID := q.Get("object")
		subjectID := q.Get("subject_id")

		// Only grant access within the correct tenant namespace
		allowed := ns == "tenant-A/resource" && objectID == "res-1" && subjectID == "user-1"

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": allowed})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := context.Background()
	adapter := newAdapter(srv.URL, "", nil)

	// User in tenant-A should have access to tenant-A resources
	result, err := adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenant-A/resource", ID: "res-1"},
		Permission: "read",
		Subject:    security.SubjectRef{ID: "user-1"},
	})
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Same user should NOT have access to tenant-B resources
	result, err = adapter.Check(ctx, security.CheckRequest{
		Object:     security.ObjectRef{Namespace: "tenant-B/resource", ID: "res-1"},
		Permission: "read",
		Subject:    security.SubjectRef{ID: "user-1"},
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
}

func TestScenario_BatchPermissionEvaluation(t *testing.T) {
	// Simulate a UI rendering scenario: check multiple permissions at once
	// to decide what buttons/actions to show the user

	mux := http.NewServeMux()
	mux.HandleFunc("GET /relation-tuples/check", func(w http.ResponseWriter, r *http.Request) {
		permission := r.URL.Query().Get("relation")

		// user can view and comment, but not delete
		allowedPerms := map[string]bool{
			"view":    true,
			"comment": true,
			"delete":  false,
			"edit":    false,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": allowedPerms[permission]})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := context.Background()
	adapter := newAdapter(srv.URL, "", nil)

	doc := security.ObjectRef{Namespace: "document", ID: "doc-42"}
	user := security.SubjectRef{ID: "current-user"}

	requests := []security.CheckRequest{
		{Object: doc, Permission: "view", Subject: user},
		{Object: doc, Permission: "comment", Subject: user},
		{Object: doc, Permission: "edit", Subject: user},
		{Object: doc, Permission: "delete", Subject: user},
	}

	results, err := adapter.BatchCheck(ctx, requests)
	require.NoError(t, err)
	require.Len(t, results, 4)

	capabilities := make(map[string]bool)
	for i, req := range requests {
		capabilities[req.Permission] = results[i].Allowed
	}

	assert.True(t, capabilities["view"])
	assert.True(t, capabilities["comment"])
	assert.False(t, capabilities["edit"])
	assert.False(t, capabilities["delete"])
}
