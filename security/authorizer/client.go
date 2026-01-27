// Package authorizer provides an Ory Keto adapter implementation for the security.Authorizer interface.
package authorizer

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/client"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/security"
)

// ketoAdapter implements the security.securityService interface using Ory Keto.
type ketoAdapter struct {
	httpClient  client.Manager
	config      config.ConfigurationAuthorization
	auditLogger security.AuditLogger
}

// NewKetoAdapter creates a new Keto adapter with the given configuration.
func NewKetoAdapter(
	cfg config.ConfigurationAuthorization,
	cl client.Manager,
	auditLogger security.AuditLogger,
) security.Authorizer {
	if auditLogger == nil {
		auditLogger = NewNoOpAuditLogger()
	}

	return &ketoAdapter{
		httpClient:  cl,
		config:      cfg,
		auditLogger: auditLogger,
	}
}

// Keto API request/response types

type ketoCheckResponse struct {
	Allowed bool `json:"allowed"`
}

type ketoRelationTuple struct {
	Namespace string          `json:"namespace"`
	Object    string          `json:"object"`
	Relation  string          `json:"relation"`
	SubjectID string          `json:"subject_id,omitempty"`
	Subject   *ketoSubjectSet `json:"subject_set,omitempty"`
}

type ketoSubjectSet struct {
	Namespace string `json:"namespace"`
	Object    string `json:"object"`
	Relation  string `json:"relation"`
}

type ketoListResponse struct {
	RelationTuples []*ketoRelationTuple `json:"relation_tuples"`
	NextPageToken  string               `json:"next_page_token,omitempty"`
}

type ketoExpandResponse struct {
	SubjectSets []struct {
		Namespace string `json:"namespace"`
		Object    string `json:"object"`
		Relation  string `json:"relation"`
	} `json:"subject_sets,omitempty"`
	SubjectIDs []string `json:"subject_ids,omitempty"`
}

// Check verifies if a subject has permission on an object.
func (k *ketoAdapter) Check(ctx context.Context, req security.CheckRequest) (security.CheckResult, error) {
	cfg := k.config

	if !cfg.AuthorizationServiceCanRead() {
		return security.CheckResult{
			Allowed:   true,
			Reason:    "keto disabled - permissive mode",
			CheckedAt: time.Now().Unix(),
		}, nil
	}

	// Build URL with query parameters
	u, err := url.Parse(cfg.GetAuthorizationServiceReadURI() + "/relation-tuples/check")
	if err != nil {
		return security.CheckResult{}, NewAuthzServiceError("check", err)
	}

	q := u.Query()
	q.Set("namespace", req.Object.Namespace)
	q.Set("object", req.Object.ID)
	q.Set("relation", req.Permission)

	if req.Subject.Relation != "" {
		// Subject set
		q.Set("subject_set.namespace", req.Subject.Namespace)
		q.Set("subject_set.object", req.Subject.ID)
		q.Set("subject_set.relation", req.Subject.Relation)
	} else {
		q.Set("subject_id", req.Subject.ID)
	}
	u.RawQuery = q.Encode()

	// Execute request with retries
	resp, err := k.httpClient.Invoke(ctx, http.MethodGet, u.String(), nil, nil)
	if err != nil {
		return security.CheckResult{}, NewAuthzServiceError("check", err)
	}

	// Keto returns 200 for allowed and 403 for denied; both carry a JSON body.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusForbidden {
		body, _ := resp.ToContent(ctx)
		return security.CheckResult{}, NewAuthzServiceError("check",
			fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body)))
	}

	var checkResp ketoCheckResponse
	err = resp.Decode(ctx, &checkResp)
	if err != nil {
		return security.CheckResult{}, NewAuthzServiceError("check", err)
	}

	result := security.CheckResult{
		Allowed:   checkResp.Allowed,
		CheckedAt: time.Now().Unix(),
	}

	if !checkResp.Allowed {
		result.Reason = "no matching relation found"
	}

	// Audit log
	err = k.auditLogger.LogDecision(ctx, req, result, nil)
	if err != nil {
		util.Log(ctx).WithError(err).Warn("failed to log authorization decision")
	}

	return result, nil
}

// BatchCheck verifies multiple permissions in one call.
func (k *ketoAdapter) BatchCheck(
	ctx context.Context,
	requests []security.CheckRequest,
) ([]security.CheckResult, error) {
	cfg := k.config

	if !cfg.AuthorizationServiceCanRead() {
		results := make([]security.CheckResult, len(requests))
		for i := range results {
			results[i] = security.CheckResult{
				Allowed:   true,
				Reason:    "keto disabled - permissive mode",
				CheckedAt: time.Now().Unix(),
			}
		}
		return results, nil
	}

	// Keto doesn't have a native batch check API, so we do individual checks
	// This could be optimized with goroutines for parallel execution
	results := make([]security.CheckResult, len(requests))
	for i, req := range requests {
		result, err := k.Check(ctx, req)
		if err != nil {
			// On error, deny access (fail closed)
			results[i] = security.CheckResult{
				Allowed:   false,
				Reason:    fmt.Sprintf("check failed: %v", err),
				CheckedAt: time.Now().Unix(),
			}
			continue
		}
		results[i] = result
	}

	return results, nil
}

// WriteTuple creates a relationship tuple.
func (k *ketoAdapter) WriteTuple(ctx context.Context, tuple security.RelationTuple) error {
	return k.WriteTuples(ctx, []security.RelationTuple{tuple})
}

// WriteTuples creates multiple relationship tuples.
// Keto's PUT endpoint accepts a single tuple per request, so each tuple
// is written individually.
func (k *ketoAdapter) WriteTuples(ctx context.Context, tuples []security.RelationTuple) error {
	cfg := k.config

	if !cfg.AuthorizationServiceCanWrite() {
		return nil
	}

	if len(tuples) == 0 {
		return nil
	}

	for _, t := range tuples {
		kt := k.toKetoTuple(t)

		resp, err := k.httpClient.Invoke(
			ctx,
			http.MethodPut,
			cfg.GetAuthorizationServiceWriteURI()+"/admin/relation-tuples",
			kt,
			nil,
		)
		if err != nil {
			return NewAuthzServiceError("write_tuples", err)
		}

		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK &&
			resp.StatusCode != http.StatusNoContent {
			respBody, _ := resp.ToContent(ctx)
			return NewAuthzServiceError("write_tuples",
				fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody)))
		}

		util.CloseAndLogOnError(ctx, resp)
	}

	return nil
}

// DeleteTuple removes a relationship tuple.
func (k *ketoAdapter) DeleteTuple(ctx context.Context, tuple security.RelationTuple) error {
	return k.DeleteTuples(ctx, []security.RelationTuple{tuple})
}

// DeleteTuples removes multiple relationship tuples atomically.
func (k *ketoAdapter) DeleteTuples(ctx context.Context, tuples []security.RelationTuple) error {
	cfg := k.config

	if !cfg.AuthorizationServiceCanWrite() {
		return nil
	}

	if len(tuples) == 0 {
		return nil
	}

	// Keto uses query parameters for delete
	for _, tuple := range tuples {
		u, err := url.Parse(cfg.GetAuthorizationServiceWriteURI() + "/admin/relation-tuples")
		if err != nil {
			return NewAuthzServiceError("delete_tuples", err)
		}

		q := u.Query()
		q.Set("namespace", tuple.Object.Namespace)
		q.Set("object", tuple.Object.ID)
		q.Set("relation", tuple.Relation)

		if tuple.Subject.Relation != "" {
			q.Set("subject_set.namespace", tuple.Subject.Namespace)
			q.Set("subject_set.object", tuple.Subject.ID)
			q.Set("subject_set.relation", tuple.Subject.Relation)
		} else {
			q.Set("subject_id", tuple.Subject.ID)
		}
		u.RawQuery = q.Encode()

		resp, err := k.httpClient.Invoke(ctx, http.MethodDelete, u.String(), nil, nil)
		if err != nil {
			return NewAuthzServiceError("delete_tuples", err)
		}

		util.CloseAndLogOnError(ctx, resp)

		// 404 is acceptable - tuple might not exist
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK &&
			resp.StatusCode != http.StatusNotFound {
			return NewAuthzServiceError("delete_tuples",
				fmt.Errorf("unexpected status %d", resp.StatusCode))
		}
	}

	return nil
}

// ListRelations returns all relations for an object.
func (k *ketoAdapter) ListRelations(ctx context.Context, object security.ObjectRef) ([]security.RelationTuple, error) {
	cfg := k.config

	if !cfg.AuthorizationServiceCanRead() {
		return []security.RelationTuple{}, nil
	}

	u, err := url.Parse(cfg.GetAuthorizationServiceReadURI() + "/relation-tuples")
	if err != nil {
		return nil, NewAuthzServiceError("list_relations", err)
	}

	q := u.Query()
	q.Set("namespace", object.Namespace)
	q.Set("object", object.ID)
	u.RawQuery = q.Encode()

	resp, err := k.httpClient.Invoke(ctx, http.MethodGet, u.String(), nil, nil)
	if err != nil {
		return nil, NewAuthzServiceError("list_relations", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := resp.ToContent(ctx)
		return nil, NewAuthzServiceError("list_relations",
			fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body)))
	}

	var listResp ketoListResponse
	err = resp.Decode(ctx, &listResp)
	if err != nil {
		return nil, NewAuthzServiceError("list_relations", err)
	}

	tuples := make([]security.RelationTuple, len(listResp.RelationTuples))
	for i, kt := range listResp.RelationTuples {
		tuples[i] = k.fromKetoTuple(kt)
	}

	return tuples, nil
}

// ListSubjectRelations returns all objects a subject has relations to.
func (k *ketoAdapter) ListSubjectRelations(
	ctx context.Context,
	subject security.SubjectRef,
	namespace string,
) ([]security.RelationTuple, error) {
	cfg := k.config

	if !cfg.AuthorizationServiceCanRead() {
		return []security.RelationTuple{}, nil
	}

	u, err := url.Parse(cfg.GetAuthorizationServiceReadURI() + "/relation-tuples")
	if err != nil {
		return nil, NewAuthzServiceError("list_subject_relations", err)
	}

	q := u.Query()
	if namespace != "" {
		q.Set("namespace", namespace)
	}
	if subject.Relation != "" {
		q.Set("subject_set.namespace", subject.Namespace)
		q.Set("subject_set.object", subject.ID)
		q.Set("subject_set.relation", subject.Relation)
	} else {
		q.Set("subject_id", subject.ID)
	}
	u.RawQuery = q.Encode()

	resp, err := k.httpClient.Invoke(ctx, http.MethodGet, u.String(), nil, nil)
	if err != nil {
		return nil, NewAuthzServiceError("list_subject_relations", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := resp.ToContent(ctx)
		return nil, NewAuthzServiceError("list_subject_relations",
			fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body)))
	}

	var listResp ketoListResponse
	err = resp.Decode(ctx, &listResp)
	if err != nil {
		return nil, NewAuthzServiceError("list_subject_relations", err)
	}

	tuples := make([]security.RelationTuple, len(listResp.RelationTuples))
	for i, kt := range listResp.RelationTuples {
		tuples[i] = k.fromKetoTuple(kt)
	}

	return tuples, nil
}

// Expand returns all subjects with a given relation.
func (k *ketoAdapter) Expand(
	ctx context.Context,
	object security.ObjectRef,
	relation string,
) ([]security.SubjectRef, error) {
	cfg := k.config

	if !cfg.AuthorizationServiceCanRead() {
		return []security.SubjectRef{}, nil
	}

	u, err := url.Parse(cfg.GetAuthorizationServiceReadURI() + "/relation-tuples/expand")
	if err != nil {
		return nil, NewAuthzServiceError("expand", err)
	}

	q := u.Query()
	q.Set("namespace", object.Namespace)
	q.Set("object", object.ID)
	q.Set("relation", relation)
	q.Set("max-depth", "3")
	u.RawQuery = q.Encode()

	resp, err := k.httpClient.Invoke(ctx, http.MethodGet, u.String(), nil, nil)
	if err != nil {
		return nil, NewAuthzServiceError("expand", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := resp.ToContent(ctx)
		return nil, NewAuthzServiceError("expand",
			fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body)))
	}

	var expandResp ketoExpandResponse
	err = resp.Decode(ctx, &expandResp)
	if err != nil {
		return nil, NewAuthzServiceError("expand", err)
	}

	subjects := make([]security.SubjectRef, 0, len(expandResp.SubjectIDs)+len(expandResp.SubjectSets))

	for _, id := range expandResp.SubjectIDs {
		subjects = append(subjects, security.SubjectRef{
			Namespace: security.NamespaceProfile,
			ID:        id,
		})
	}

	for _, ss := range expandResp.SubjectSets {
		subjects = append(subjects, security.SubjectRef{
			Namespace: ss.Namespace,
			ID:        ss.Object,
			Relation:  ss.Relation,
		})
	}

	return subjects, nil
}

// Helper methods

func (k *ketoAdapter) toKetoTuple(t security.RelationTuple) *ketoRelationTuple {
	kt := &ketoRelationTuple{
		Namespace: t.Object.Namespace,
		Object:    t.Object.ID,
		Relation:  t.Relation,
	}

	if t.Subject.Relation != "" {
		kt.Subject = &ketoSubjectSet{
			Namespace: t.Subject.Namespace,
			Object:    t.Subject.ID,
			Relation:  t.Subject.Relation,
		}
	} else {
		kt.SubjectID = t.Subject.ID
	}

	return kt
}

func (k *ketoAdapter) fromKetoTuple(kt *ketoRelationTuple) security.RelationTuple {
	t := security.RelationTuple{
		Object: security.ObjectRef{
			Namespace: kt.Namespace,
			ID:        kt.Object,
		},
		Relation: kt.Relation,
	}

	if kt.Subject != nil {
		t.Subject = security.SubjectRef{
			Namespace: kt.Subject.Namespace,
			ID:        kt.Subject.Object,
			Relation:  kt.Subject.Relation,
		}
	} else {
		t.Subject = security.SubjectRef{
			Namespace: security.NamespaceProfile,
			ID:        kt.SubjectID,
		}
	}

	return t
}
