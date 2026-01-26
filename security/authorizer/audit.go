package authorizer

import (
	"context"
	"math/rand/v2"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/security"
)

// auditLogger implements the AuditLogger interface.
type auditLogger struct {
	sampleRate float64
	enabled    bool
}

// AuditLoggerConfig holds configuration for the audit logger.
type AuditLoggerConfig struct {
	// SampleRate is the fraction of decisions to log (0.0 to 1.0).
	SampleRate float64
}

// NewAuditLogger creates a new AuditLogger with the given configuration.
func NewAuditLogger(config AuditLoggerConfig) security.AuditLogger {
	if config.SampleRate <= 0 {
		config.SampleRate = 1.0
	}
	if config.SampleRate > 1.0 {
		config.SampleRate = 1.0
	}

	return &auditLogger{
		sampleRate: config.SampleRate,
	}
}

// LogDecision logs an authorization decision for security audit.
func (a *auditLogger) LogDecision(
	ctx context.Context,
	req security.CheckRequest,
	result security.CheckResult,
	metadata map[string]string,
) error {
	if !a.enabled {
		return nil
	}

	// Sample rate check
	if a.sampleRate < 1.0 && rand.Float64() > a.sampleRate { //nolint:gosec // sample rate doesn't need crypto rand
		return nil
	}

	fields := map[string]any{
		"object_namespace": req.Object.Namespace,
		"object_id":        req.Object.ID,
		"permission":       req.Permission,
		"subject_ns":       req.Subject.Namespace,
		"subject_id":       req.Subject.ID,
		"allowed":          result.Allowed,
		"checked_at":       result.CheckedAt,
	}

	if result.Reason != "" {
		fields["reason"] = result.Reason
	}

	if req.Subject.Relation != "" {
		fields["subject_relation"] = req.Subject.Relation
	}

	// Add custom metadata
	for k, v := range metadata {
		fields["meta_"+k] = v
	}

	log := util.Log(ctx).WithFields(fields)

	if result.Allowed {
		log.Debug("authorization decision: allowed")
	} else {
		log.Info("authorization decision: denied")
	}

	return nil
}

// NoOpAuditLogger is an audit logger that does nothing.
type NoOpAuditLogger struct{}

// LogDecision implements AuditLogger but does nothing.
func (n *NoOpAuditLogger) LogDecision(
	_ context.Context,
	_ security.CheckRequest,
	_ security.CheckResult,
	_ map[string]string,
) error {
	return nil
}

// NewNoOpAuditLogger creates a new no-op audit logger.
func NewNoOpAuditLogger() security.AuditLogger {
	return &NoOpAuditLogger{}
}
