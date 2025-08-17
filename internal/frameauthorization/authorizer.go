package frameauthorization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// authorizer implements the Authorizer interface
type authorizer struct {
	config     Config
	httpClient HTTPClient
	logger     Logger
}

// NewAuthorizer creates a new authorizer instance
func NewAuthorizer(config Config, httpClient HTTPClient, logger Logger) Authorizer {
	return &authorizer{
		config:     config,
		httpClient: httpClient,
		logger:     logger,
	}
}

// IsEnabled returns whether authorization is enabled
func (a *authorizer) IsEnabled() bool {
	return a.config != nil && a.config.GetAuthorizationServiceReadURI() != ""
}

// HasAccess checks if a subject can perform an action on a resource
func (a *authorizer) HasAccess(ctx context.Context, action string, subject string) (bool, error) {
	if !a.IsEnabled() {
		if a.logger != nil {
			a.logger.Debug("Authorization is disabled, allowing access")
		}
		return true, nil
	}

	// Get claims from context
	claims := GetClaimsFromContext(ctx)
	if claims == nil {
		return false, errors.New("only authenticated requests should be used to check authorization")
	}

	// Prepare authorization request payload
	payload := map[string]any{
		"namespace":  claims.GetTenantID(),
		"object":     claims.GetPartitionID(),
		"relation":   action,
		"subject_id": subject,
	}

	// Make authorization request
	status, result, err := a.httpClient.InvokeRestService(ctx, http.MethodPost,
		a.config.GetAuthorizationServiceReadURI(), payload, nil)
	if err != nil {
		a.logger.WithError(err).WithField("action", action).WithField("subject", subject).
			Error("Failed to invoke authorization service")
		return false, err
	}

	// Check response status
	if status > 299 || status < 200 {
		errMsg := fmt.Sprintf("invalid response status %d had message %s", status, string(result))
		a.logger.WithField("status", status).WithField("response", string(result)).
			WithField("action", action).WithField("subject", subject).
			Error("Authorization service returned error status")
		return false, fmt.Errorf("%s", errMsg)
	}

	// Parse response
	var response map[string]any
	err = json.Unmarshal(result, &response)
	if err != nil {
		a.logger.WithError(err).WithField("response", string(result)).
			Error("Failed to parse authorization service response")
		return false, err
	}

	// Check if access is allowed
	if val, allowedExists := response["allowed"]; allowedExists {
		if boolVal, typeOK := val.(bool); typeOK && boolVal {
			a.logger.WithField("action", action).WithField("subject", subject).
				Debug("Authorization granted")
			return true, nil
		}
	}

	a.logger.WithField("action", action).WithField("subject", subject).
		Debug("Authorization denied")
	return false, nil
}

// GetClaimsFromContext extracts claims from context
// This is a placeholder function that should be implemented to extract
// authentication claims from the context
func GetClaimsFromContext(ctx context.Context) ClaimsProvider {
	// This would typically extract claims from context using the frameauth package
	// For now, we'll return a placeholder implementation
	return &contextClaims{ctx: ctx}
}

// contextClaims is a placeholder implementation of ClaimsProvider
type contextClaims struct {
	ctx context.Context
}

func (c *contextClaims) GetTenantID() string {
	// This would extract tenant ID from context claims
	// Placeholder implementation
	return ""
}

func (c *contextClaims) GetPartitionID() string {
	// This would extract partition ID from context claims
	// Placeholder implementation
	return ""
}
