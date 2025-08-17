package frameauth

import (
	"context"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

func (c contextKey) String() string {
	return "frameauth/" + string(c)
}

const (
	ctxKeyAuthenticationClaim       = contextKey("authenticationClaimKey")
	ctxKeySkipTenancyCheckOnClaim   = contextKey("skipTenancyCheckOnClaimKey")
	ctxKeyAuthenticationJwt         = contextKey("authenticationJwtKey")
)

// AuthenticationClaims defines the structure for JWT claims, embedding jwt.RegisteredClaims
// to include standard fields like expiry time, and adding custom claims.
type AuthenticationClaims struct {
	Ext         map[string]any `json:"ext,omitempty"`
	TenantID    string         `json:"tenant_id,omitempty"`
	PartitionID string         `json:"partition_id,omitempty"`
	AccessID    string         `json:"access_id,omitempty"`
	ContactID   string         `json:"contact_id,omitempty"`
	SessionID   string         `json:"session_id,omitempty"`
	DeviceID    string         `json:"device_id,omitempty"`
	ServiceName string         `json:"service_name,omitempty"`
	Roles       []string       `json:"roles,omitempty"`
	jwt.RegisteredClaims
}

// GetTenantID returns the tenant ID from claims or ext fields
func (a *AuthenticationClaims) GetTenantID() string {
	result := a.TenantID
	if result != "" {
		return result
	}
	val, ok := a.Ext["tenant_id"]
	if !ok {
		return ""
	}

	result, ok = val.(string)
	if !ok {
		return ""
	}

	return result
}

// GetPartitionID returns the partition ID from claims or ext fields
func (a *AuthenticationClaims) GetPartitionID() string {
	result := a.PartitionID
	if result != "" {
		return result
	}
	val, ok := a.Ext["partition_id"]
	if !ok {
		return ""
	}

	result, ok = val.(string)
	if !ok {
		return ""
	}

	return result
}

// GetAccessID returns the access ID from claims or ext fields
func (a *AuthenticationClaims) GetAccessID() string {
	result := a.AccessID
	if result != "" {
		return result
	}
	val, ok := a.Ext["access_id"]
	if !ok {
		return ""
	}

	result, ok = val.(string)
	if !ok {
		return ""
	}

	return result
}

// GetContactID returns the contact ID from claims or ext fields
func (a *AuthenticationClaims) GetContactID() string {
	result := a.ContactID
	if result != "" {
		return result
	}
	val, ok := a.Ext["contact_id"]
	if !ok {
		return ""
	}

	result, ok = val.(string)
	if !ok {
		return ""
	}

	return result
}

// GetSessionID returns the session ID from claims or ext fields
func (a *AuthenticationClaims) GetSessionID() string {
	result := a.SessionID
	if result != "" {
		return result
	}
	val, ok := a.Ext["session_id"]
	if !ok {
		return ""
	}

	result, ok = val.(string)
	if !ok {
		return ""
	}

	return result
}

// GetDeviceID returns the device ID from claims or ext fields
func (a *AuthenticationClaims) GetDeviceID() string {
	result := a.DeviceID
	if result != "" {
		return result
	}
	val, ok := a.Ext["device_id"]
	if !ok {
		return ""
	}

	result, ok = val.(string)
	if !ok {
		return ""
	}

	return result
}

// GetRoles returns the roles from claims or ext fields
func (a *AuthenticationClaims) GetRoles() []string {
	var result = a.Roles
	if len(result) > 0 {
		return result
	}

	roles, ok := a.Ext["roles"]
	if !ok {
		roles, ok = a.Ext["role"]
		if !ok {
			// Return empty slice instead of nil
			if result == nil {
				return []string{}
			}
			return result
		}
	}

	roleStr, ok2 := roles.(string)
	if ok2 {
		result = append(result, strings.Split(roleStr, ",")...)
	}

	// Return empty slice instead of nil if result is still nil
	if result == nil {
		return []string{}
	}
	return result
}

// GetServiceName returns the service name from claims or ext fields
func (a *AuthenticationClaims) GetServiceName() string {
	result := a.ServiceName
	if result != "" {
		return result
	}
	val, ok := a.Ext["service_name"]
	if !ok {
		return ""
	}

	result, ok = val.(string)
	if !ok {
		return ""
	}

	return result
}

// isInternalSystem checks if the claims represent an internal system
func (a *AuthenticationClaims) isInternalSystem() bool {
	roles := a.GetRoles()
	if len(roles) == 1 {
		if strings.HasPrefix(roles[0], "system_internal") {
			return true
		}
	}

	return false
}

// AsMetadata creates a string map to be used as metadata in queue data
func (a *AuthenticationClaims) AsMetadata() map[string]string {
	m := make(map[string]string)
	m["sub"] = a.Subject
	m["tenant_id"] = a.GetTenantID()
	m["partition_id"] = a.GetPartitionID()
	m["access_id"] = a.GetAccessID()
	m["contact_id"] = a.GetContactID()
	m["device_id"] = a.GetDeviceID()
	m["roles"] = strings.Join(a.GetRoles(), ",")
	return m
}

// ClaimsToContext adds authentication claims to the current supplied context
func (a *AuthenticationClaims) ClaimsToContext(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, ctxKeyAuthenticationClaim, a)

	if a.isInternalSystem() {
		ctx = SkipTenancyChecksOnClaims(ctx)
	}

	return ctx
}

// SkipTenancyChecksOnClaims removes authentication claims from the current supplied context
func SkipTenancyChecksOnClaims(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeySkipTenancyCheckOnClaim, true)
}

// IsTenancyChecksOnClaimSkipped checks if tenancy checks are skipped
func IsTenancyChecksOnClaimSkipped(ctx context.Context) bool {
	isSkipped, ok := ctx.Value(ctxKeySkipTenancyCheckOnClaim).(bool)
	if !ok {
		return false
	}
	return isSkipped
}

// ClaimsFromContext extracts authentication claims from the supplied context if any exist
func ClaimsFromContext(ctx context.Context) *AuthenticationClaims {
	authenticationClaims, ok := ctx.Value(ctxKeyAuthenticationClaim).(*AuthenticationClaims)
	if !ok {
		return nil
	}

	return authenticationClaims
}

// ClaimsFromMap extracts authentication claims from the supplied map if they exist
func ClaimsFromMap(m map[string]string) *AuthenticationClaims {
	// Extract required fields and return nil if any are missing
	sub, okSubject := m["sub"]
	tenantID, okTenant := m["tenant_id"]
	partitionID, okPartition := m["partition_id"]

	if !okSubject && !okTenant && !okPartition {
		return nil
	}

	// Initialize AuthenticationClaims with required fields
	claims := &AuthenticationClaims{
		TenantID:    tenantID,
		PartitionID: partitionID,
		Ext:         make(map[string]any),
	}
	claims.Subject = sub

	for key, val := range m {
		switch key {
		case "access_id":
			claims.AccessID = val
		case "contact_id":
			claims.ContactID = val
		case "device_id":
			claims.DeviceID = val
		case "roles":
			claims.Ext[key] = strings.Split(val, ",")
		default:
			// Skip primary values ("sub", "tenant_id", "partition_id")
			if key == "sub" || key == "tenant_id" || key == "partition_id" {
				continue
			}
			// Add other fields to Ext
			claims.Ext[key] = val
		}
	}

	return claims
}

// jwtToContext adds authentication jwt to the current supplied context
func jwtToContext(ctx context.Context, jwt string) context.Context {
	return context.WithValue(ctx, ctxKeyAuthenticationJwt, jwt)
}

// JwtFromContext extracts authentication jwt from the supplied context if any exist
func JwtFromContext(ctx context.Context) string {
	jwtString, ok := ctx.Value(ctxKeyAuthenticationJwt).(string)
	if !ok {
		return ""
	}

	return jwtString
}
