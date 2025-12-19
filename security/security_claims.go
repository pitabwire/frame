package security

import (
	"context"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pitabwire/util"
)

type contextKey string

func (c contextKey) String() string {
	return "frame/security/" + string(c)
}

const ctxKeyAuthenticationClaim = contextKey("authenticationClaimKey")
const ctxKeySkipTenancyCheckOnClaim = contextKey("skipTenancyCheckOnClaimKey")
const ctxKeyAuthenticationJwt = contextKey("authenticationJwtKey")

// JwtToContext adds authentication jwt to the current supplied context.
func JwtToContext(ctx context.Context, jwt string) context.Context {
	return context.WithValue(ctx, ctxKeyAuthenticationJwt, jwt)
}

// JwtFromContext extracts authentication jwt from the supplied context if any exist.
func JwtFromContext(ctx context.Context) string {
	jwtString, ok := ctx.Value(ctxKeyAuthenticationJwt).(string)
	if !ok {
		return ""
	}

	return jwtString
}

// AuthenticationClaims defines the structure for JWT claims, embedding jwt.StandardClaims
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

func (a *AuthenticationClaims) GetProfileID() string {
	result, _ := a.GetSubject()

	return result
}

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

func (a *AuthenticationClaims) GetRoles() []string {
	var result = a.Roles
	if len(result) > 0 {
		return result
	}

	roles, ok := a.Ext["roles"]
	if !ok {
		roles, ok = a.Ext["role"]
		if !ok {
			return result
		}
	}

	roleStr, ok2 := roles.(string)
	if ok2 {
		result = append(result, strings.Split(roleStr, ",")...)
	}

	return result
}

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

func (a *AuthenticationClaims) isInternalSystem() bool {
	roles := a.GetRoles()
	if len(roles) == 1 {
		if strings.HasPrefix(roles[0], "system_internal") {
			return true
		}
	}

	return false
}

// AsMetadata Creates a string map to be used as metadata in queue data.
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

// ClaimsToContext adds authentication claims to the current supplied context.
func (a *AuthenticationClaims) ClaimsToContext(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, ctxKeyAuthenticationClaim, a)

	if a.isInternalSystem() {
		ctx = SkipTenancyChecksOnClaims(ctx)
	}

	return ctx
}

// SkipTenancyChecksOnClaims removes authentication claims from the current supplied context.
func SkipTenancyChecksOnClaims(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeySkipTenancyCheckOnClaim, true)
}

func SkipTenancyChecksFromMap(ctx context.Context, m map[string]string) context.Context {
	check, ok := m["skip_tenancy_check"]
	if ok && check == "true" {
		return SkipTenancyChecksOnClaims(ctx)
	}

	return ctx
}

func SkipTenancyChecksToMap(ctx context.Context, m map[string]string) map[string]string {
	if !IsTenancyChecksOnClaimSkipped(ctx) {
		m["skip_tenancy_check"] = "true"
	}
	return m
}

func IsTenancyChecksOnClaimSkipped(ctx context.Context) bool {
	isSkipped, ok := ctx.Value(ctxKeySkipTenancyCheckOnClaim).(bool)
	if !ok {
		return false
	}
	return isSkipped
}

// ClaimsFromContext extracts authentication claims from the supplied context if any exist.
// For internal systems, the returned claims are enriched with tenancy data from secondary claims.
func ClaimsFromContext(ctx context.Context) *AuthenticationClaims {
	authenticationClaims, ok := ctx.Value(ctxKeyAuthenticationClaim).(*AuthenticationClaims)
	if !ok {
		return nil
	}

	if authenticationClaims.isInternalSystem() {
		secondaryClaims := util.GetTenancy(ctx)
		if secondaryClaims != nil {
			// Return enriched copy to avoid mutating the original claims in context
			enriched := *authenticationClaims
			enriched.TenantID = secondaryClaims.GetTenantID()
			enriched.PartitionID = secondaryClaims.GetPartitionID()
			enriched.AccessID = secondaryClaims.GetAccessID()
			return &enriched
		}
	}

	return authenticationClaims
}

// ClaimsFromMap extracts authentication claims from the supplied map if they exist.
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

// SetupSecondaryClaims internal services act on behalf of different users
// Although they have their claims in place there may be situations where there is need to login as
// This is where secondary claims come into play and implementing systems can decide to use the secondary claims
// This should be done with very high caution though.
func SetupSecondaryClaims(
	ctx context.Context,
	tenantID, partitionID, profileID, accessID,
	contactID, sessionID, deviceID, roles string,
) context.Context {
	claims := ClaimsFromContext(ctx)

	// If no claims or not an internal system, no padding is needed.
	if claims == nil || !claims.isInternalSystem() || tenantID == "" || partitionID == "" {
		return ctx
	}

	secondaryClaims := &AuthenticationClaims{
		TenantID:    tenantID,
		PartitionID: partitionID,
		Ext:         make(map[string]any),
	}
	secondaryClaims.Subject = claims.Subject

	secondaryClaims.TenantID = tenantID
	secondaryClaims.PartitionID = partitionID
	secondaryClaims.Subject = profileID
	secondaryClaims.AccessID = accessID
	secondaryClaims.ContactID = contactID
	secondaryClaims.SessionID = sessionID
	secondaryClaims.DeviceID = deviceID
	secondaryClaims.Roles = strings.Split(roles, ",")

	return util.SetTenancy(ctx, secondaryClaims)
}
