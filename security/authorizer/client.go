// Package authorizer provides an Ory Keto adapter implementation for the security.Authorizer interface.
package authorizer

import (
	"context"
	"fmt"
	"time"

	rts "github.com/ory/keto/proto/ory/keto/relation_tuples/v1alpha2"
	"github.com/pitabwire/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/security"
)

// ketoAdapter implements the security.Authorizer interface using Ory Keto gRPC.
type ketoAdapter struct {
	readConn     *grpc.ClientConn
	writeConn    *grpc.ClientConn
	checkClient  rts.CheckServiceClient
	readClient   rts.ReadServiceClient
	writeClient  rts.WriteServiceClient
	expandClient rts.ExpandServiceClient
	config       config.ConfigurationAuthorization
	auditLogger  security.AuditLogger
}

// NewKetoAdapter creates a new Keto adapter with the given configuration.
func NewKetoAdapter(
	cfg config.ConfigurationAuthorization,
	auditLogger security.AuditLogger,
) security.Authorizer {
	if auditLogger == nil {
		auditLogger = NewNoOpAuditLogger()
	}

	adapter := &ketoAdapter{
		config:      cfg,
		auditLogger: auditLogger,
	}

	if cfg.AuthorizationServiceCanRead() {
		readConn, err := grpc.NewClient(
			cfg.GetAuthorizationServiceReadURI(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err == nil {
			adapter.readConn = readConn
			adapter.checkClient = rts.NewCheckServiceClient(readConn)
			adapter.readClient = rts.NewReadServiceClient(readConn)
			adapter.expandClient = rts.NewExpandServiceClient(readConn)
		}
	}

	if cfg.AuthorizationServiceCanWrite() {
		writeConn, err := grpc.NewClient(
			cfg.GetAuthorizationServiceWriteURI(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err == nil {
			adapter.writeConn = writeConn
			adapter.writeClient = rts.NewWriteServiceClient(writeConn)
		}
	}

	return adapter
}

// Close releases gRPC connections.
func (k *ketoAdapter) Close() {
	if k.readConn != nil {
		_ = k.readConn.Close()
	}
	if k.writeConn != nil {
		_ = k.writeConn.Close()
	}
}

// Check verifies if a subject has permission on an object.
func (k *ketoAdapter) Check(ctx context.Context, req security.CheckRequest) (security.CheckResult, error) {
	if !k.config.AuthorizationServiceCanRead() {
		return security.CheckResult{
			Allowed:   true,
			Reason:    "keto disabled - permissive mode",
			CheckedAt: time.Now().Unix(),
		}, nil
	}

	tuple := &rts.RelationTuple{
		Namespace: req.Object.Namespace,
		Object:    req.Object.ID,
		Relation:  req.Permission,
		Subject:   toKetoSubject(req.Subject),
	}

	resp, err := k.checkClient.Check(ctx, &rts.CheckRequest{
		Tuple: tuple,
	})
	if err != nil {
		return security.CheckResult{}, NewAuthzServiceError("check", err)
	}

	result := security.CheckResult{
		Allowed:   resp.Allowed,
		CheckedAt: time.Now().Unix(),
	}

	if !resp.Allowed {
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
	if !k.config.AuthorizationServiceCanRead() {
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

	// Use individual checks since BatchCheck may not be available in all Keto versions
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

// WriteTuples creates multiple relationship tuples in a single transaction.
func (k *ketoAdapter) WriteTuples(ctx context.Context, tuples []security.RelationTuple) error {
	if !k.config.AuthorizationServiceCanWrite() {
		return nil
	}

	if len(tuples) == 0 {
		return nil
	}

	deltas := make([]*rts.RelationTupleDelta, len(tuples))
	for i, t := range tuples {
		deltas[i] = &rts.RelationTupleDelta{
			Action:        rts.RelationTupleDelta_ACTION_INSERT,
			RelationTuple: toKetoTuple(t),
		}
	}

	_, err := k.writeClient.TransactRelationTuples(ctx, &rts.TransactRelationTuplesRequest{
		RelationTupleDeltas: deltas,
	})
	if err != nil {
		return NewAuthzServiceError("write_tuples", err)
	}

	return nil
}

// DeleteTuple removes a relationship tuple.
func (k *ketoAdapter) DeleteTuple(ctx context.Context, tuple security.RelationTuple) error {
	return k.DeleteTuples(ctx, []security.RelationTuple{tuple})
}

// DeleteTuples removes multiple relationship tuples in a single transaction.
func (k *ketoAdapter) DeleteTuples(ctx context.Context, tuples []security.RelationTuple) error {
	if !k.config.AuthorizationServiceCanWrite() {
		return nil
	}

	if len(tuples) == 0 {
		return nil
	}

	deltas := make([]*rts.RelationTupleDelta, len(tuples))
	for i, t := range tuples {
		deltas[i] = &rts.RelationTupleDelta{
			Action:        rts.RelationTupleDelta_ACTION_DELETE,
			RelationTuple: toKetoTuple(t),
		}
	}

	_, err := k.writeClient.TransactRelationTuples(ctx, &rts.TransactRelationTuplesRequest{
		RelationTupleDeltas: deltas,
	})
	if err != nil {
		return NewAuthzServiceError("delete_tuples", err)
	}

	return nil
}

// ListRelations returns all relations for an object.
func (k *ketoAdapter) ListRelations(ctx context.Context, object security.ObjectRef) ([]security.RelationTuple, error) {
	if !k.config.AuthorizationServiceCanRead() {
		return []security.RelationTuple{}, nil
	}

	resp, err := k.readClient.ListRelationTuples(ctx, &rts.ListRelationTuplesRequest{
		RelationQuery: &rts.RelationQuery{
			Namespace: &object.Namespace,
			Object:    &object.ID,
		},
	})
	if err != nil {
		return nil, NewAuthzServiceError("list_relations", err)
	}

	tuples := make([]security.RelationTuple, len(resp.RelationTuples))
	for i, kt := range resp.RelationTuples {
		tuples[i] = fromKetoTuple(kt)
	}

	return tuples, nil
}

// ListSubjectRelations returns all objects a subject has relations to.
func (k *ketoAdapter) ListSubjectRelations(
	ctx context.Context,
	subject security.SubjectRef,
	namespace string,
) ([]security.RelationTuple, error) {
	if !k.config.AuthorizationServiceCanRead() {
		return []security.RelationTuple{}, nil
	}

	query := &rts.RelationQuery{
		Subject: toKetoSubject(subject),
	}
	if namespace != "" {
		query.Namespace = &namespace
	}

	resp, err := k.readClient.ListRelationTuples(ctx, &rts.ListRelationTuplesRequest{
		RelationQuery: query,
	})
	if err != nil {
		return nil, NewAuthzServiceError("list_subject_relations", err)
	}

	tuples := make([]security.RelationTuple, len(resp.RelationTuples))
	for i, kt := range resp.RelationTuples {
		tuples[i] = fromKetoTuple(kt)
	}

	return tuples, nil
}

// Expand returns all subjects with a given relation.
func (k *ketoAdapter) Expand(
	ctx context.Context,
	object security.ObjectRef,
	relation string,
) ([]security.SubjectRef, error) {
	if !k.config.AuthorizationServiceCanRead() {
		return []security.SubjectRef{}, nil
	}

	resp, err := k.expandClient.Expand(ctx, &rts.ExpandRequest{
		Subject:  rts.NewSubjectSet(object.Namespace, object.ID, relation),
		MaxDepth: 3,
	})
	if err != nil {
		return nil, NewAuthzServiceError("expand", err)
	}

	subjects := collectSubjects(resp.Tree)
	return subjects, nil
}

// collectSubjects recursively walks a SubjectTree and returns a flat list of SubjectRef.
func collectSubjects(tree *rts.SubjectTree) []security.SubjectRef {
	if tree == nil {
		return nil
	}

	var subjects []security.SubjectRef

	if tree.Tuple != nil && tree.Tuple.Subject != nil {
		switch s := tree.Tuple.Subject.Ref.(type) {
		case *rts.Subject_Id:
			subjects = append(subjects, security.SubjectRef{
				Namespace: security.NamespaceProfile,
				ID:        s.Id,
			})
		case *rts.Subject_Set:
			subjects = append(subjects, security.SubjectRef{
				Namespace: s.Set.Namespace,
				ID:        s.Set.Object,
				Relation:  s.Set.Relation,
			})
		}
	}

	for _, child := range tree.Children {
		subjects = append(subjects, collectSubjects(child)...)
	}

	return subjects
}

// Helper functions

func toKetoSubject(s security.SubjectRef) *rts.Subject {
	if s.Relation != "" {
		return rts.NewSubjectSet(s.Namespace, s.ID, s.Relation)
	}
	return rts.NewSubjectID(s.ID)
}

func toKetoTuple(t security.RelationTuple) *rts.RelationTuple {
	return &rts.RelationTuple{
		Namespace: t.Object.Namespace,
		Object:    t.Object.ID,
		Relation:  t.Relation,
		Subject:   toKetoSubject(t.Subject),
	}
}

func fromKetoTuple(kt *rts.RelationTuple) security.RelationTuple {
	t := security.RelationTuple{
		Object: security.ObjectRef{
			Namespace: kt.Namespace,
			ID:        kt.Object,
		},
		Relation: kt.Relation,
	}

	if kt.Subject != nil {
		switch s := kt.Subject.Ref.(type) {
		case *rts.Subject_Id:
			t.Subject = security.SubjectRef{
				Namespace: security.NamespaceProfile,
				ID:        s.Id,
			}
		case *rts.Subject_Set:
			t.Subject = security.SubjectRef{
				Namespace: s.Set.Namespace,
				ID:        s.Set.Object,
				Relation:  s.Set.Relation,
			}
		}
	}

	return t
}
