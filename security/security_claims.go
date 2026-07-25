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

const (
	ctxKeyAuthenticationClaim     = contextKey("authenticationClaimKey")
	ctxKeySkipTenancyCheckOnClaim = contextKey("skipTenancyCheckOnClaimKey")
	ctxKeyAuthenticationJwt       = contextKey("authenticationJwtKey")

	ConstantSystemInternalRole = "internal"
)

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
//
// Platform identity invariant:
//
//	JWT sub === profile_id always.
//	The acting principal for authorization is the profile. OAuth client_id only
//	identifies the client/partition at token issuance — never the actor.
//
// Hydra client_credentials currently writes wire sub=client_id and cannot
// override sub from the token hook. Token enrichment still sets profile_id in
// claims; NormalizeIdentity() rewrites RegisteredClaims.Subject to profile_id
// immediately after authentication so GetSubject() / GetProfileID() and all
// ReBAC checkers observe sub=profile_id.
type AuthenticationClaims struct {
	Ext         map[string]any `json:"ext,omitempty"`
	TenantID    string         `json:"tenant_id,omitempty"`
	PartitionID string         `json:"partition_id,omitempty"`
	// ProfileID is the acting profile (top-level JWT claim). After
	// NormalizeIdentity, this matches RegisteredClaims.Subject.
	ProfileID   string   `json:"profile_id,omitempty"`
	AccessID    string   `json:"access_id,omitempty"`
	ContactID   string   `json:"contact_id,omitempty"`
	SessionID   string   `json:"session_id,omitempty"`
	DeviceID    string   `json:"device_id,omitempty"`
	ServiceName string   `json:"service_name,omitempty"`
	Roles       []string `json:"roles,omitempty"`
	jwt.RegisteredClaims
}

// NormalizeIdentity enforces JWT sub === profile_id.
//
// When the wire token has sub=client_id (Hydra client_credentials) but carries
// profile_id in claims/ext, Subject is rewritten to that profile_id. When only
// sub is present (user tokens), ProfileID is filled from sub.
func (a *AuthenticationClaims) NormalizeIdentity() {
	if a == nil {
		return
	}
	if pid := a.profileIDFromClaims(); pid != "" {
		a.ProfileID = pid
		a.Subject = pid
		return
	}
	if sub := strings.TrimSpace(a.Subject); sub != "" {
		a.ProfileID = sub
	}
}

// profileIDFromClaims returns profile_id from top-level or ext claims only
// (does not fall back to JWT sub).
func (a *AuthenticationClaims) profileIDFromClaims() string {
	if a == nil {
		return ""
	}
	if id := strings.TrimSpace(a.ProfileID); id != "" {
		return id
	}
	if a.Ext != nil {
		if v, ok := a.Ext["profile_id"].(string); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (a *AuthenticationClaims) GetTenantID() string {
	if a == nil {
		return ""
	}
	result := strings.TrimSpace(a.TenantID)
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

	return strings.TrimSpace(result)
}

func (a *AuthenticationClaims) GetPartitionID() string {
	if a == nil {
		return ""
	}
	result := strings.TrimSpace(a.PartitionID)
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

	return strings.TrimSpace(result)
}

// GetPartitionIDs returns every partition this principal can access:
// the primary PartitionID plus any extras carried in Ext["partition_ids"]
// (as a []string, []any, or comma-separated string). The list is
// trimmed, deduplicated, and the primary id appears first. Returns an
// empty slice when no partitions are set.
//
// Use this when callers may legitimately span multiple partitions —
// e.g. a SACCO operator with access to several branches, or a
// reporting analyst aggregating across groups. Single-partition
// callers continue to work: the returned slice has one element.
func (a *AuthenticationClaims) GetPartitionIDs() []string {
	if a == nil {
		return nil
	}
	primary := a.GetPartitionID()
	additional := extractAdditionalPartitionIDs(a.Ext["partition_ids"])

	result := make([]string, 0, len(additional)+1)
	seen := make(map[string]struct{}, len(additional)+1)
	if primary != "" {
		result = append(result, primary)
		seen[primary] = struct{}{}
	}
	for _, p := range additional {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		result = append(result, p)
		seen[p] = struct{}{}
	}
	return result
}

// extractAdditionalPartitionIDs normalises the Ext["partition_ids"]
// payload (which may arrive as []string, []any, or a comma-separated
// string thanks to JWT marshallers) into a plain []string slice.
// Each entry is trimmed; empty values are dropped.
func extractAdditionalPartitionIDs(raw any) []string {
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, isStr := item.(string); isStr {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					out = append(out, trimmed)
				}
			}
		}
		return out
	case string:
		out := make([]string, 0)
		for _, s := range strings.Split(v, ",") {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	}
	return nil
}

// GetProfileID returns the acting profile identity (JWT sub after normalize).
//
// Platform invariant: sub === profile_id. Prefer an explicit profile_id claim
// (top-level or ext) when present so machine tokens with wire sub=client_id
// still resolve to the bot profile; otherwise use JWT sub.
func (a *AuthenticationClaims) GetProfileID() string {
	if a == nil {
		return ""
	}
	if id := a.profileIDFromClaims(); id != "" {
		return id
	}
	return strings.TrimSpace(a.Subject)
}

func (a *AuthenticationClaims) GetAccessID() string {
	if a == nil {
		return ""
	}
	result := strings.TrimSpace(a.AccessID)
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

	return strings.TrimSpace(result)
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
	for _, role := range roles {
		if strings.EqualFold(ConstantSystemInternalRole, role) {
			return true
		}
	}
	return false
}

// IsInternalSystem reports whether the claims belong to an internal system caller.
func (a *AuthenticationClaims) IsInternalSystem() bool {
	return a.isInternalSystem()
}

// AsMetadata Creates a string map to be used as metadata in queue data.
// Multi-partition principals encode extra partitions in "partition_ids"
// (comma-separated, excluding the primary) so ClaimsFromMap can restore
// the full set on the consumer side.
func (a *AuthenticationClaims) AsMetadata() map[string]string {
	m := make(map[string]string)
	if a == nil {
		return m
	}
	m["sub"] = a.Subject
	m["tenant_id"] = a.GetTenantID()
	m["partition_id"] = a.GetPartitionID()
	if extras := partitionIDsForMetadata(a.GetPartitionIDs(), a.GetPartitionID()); extras != "" {
		m["partition_ids"] = extras
	}
	m["access_id"] = a.GetAccessID()
	m["contact_id"] = a.GetContactID()
	m["device_id"] = a.GetDeviceID()
	m["roles"] = strings.Join(a.GetRoles(), ",")
	return m
}

// partitionIDsForMetadata returns extra partition IDs beyond the primary,
// joined with ','. Empty when there is only a primary (or none).
func partitionIDsForMetadata(all []string, primary string) string {
	if len(all) == 0 {
		return ""
	}
	extras := make([]string, 0, len(all))
	for _, p := range all {
		if p == "" || p == primary {
			continue
		}
		extras = append(extras, p)
	}
	if len(extras) == 0 {
		return ""
	}
	return strings.Join(extras, ",")
}

// ClaimsToContext adds authentication claims to the current supplied context.
// It normalizes identity first so sub === profile_id for all consumers.
func (a *AuthenticationClaims) ClaimsToContext(ctx context.Context) context.Context {
	if a != nil {
		a.NormalizeIdentity()
	}
	ctx = context.WithValue(ctx, ctxKeyAuthenticationClaim, a)

	if a != nil && a.isInternalSystem() {
		ctx = SkipTenancyChecksOnClaims(ctx)
	}

	return ctx
}

// SkipTenancyChecksOnClaims removes authentication claims from the current supplied context.
func SkipTenancyChecksOnClaims(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeySkipTenancyCheckOnClaim, true)
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
// Supports the keys produced by AsMetadata, including multi-partition
// "partition_ids" (comma-separated extras restored into Ext).
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
		TenantID:    strings.TrimSpace(tenantID),
		PartitionID: strings.TrimSpace(partitionID),
		Ext:         make(map[string]any),
	}
	claims.Subject = strings.TrimSpace(sub)

	for key, val := range m {
		switch key {
		case "profile_id":
			claims.ProfileID = strings.TrimSpace(val)
		case "access_id":
			claims.AccessID = strings.TrimSpace(val)
		case "contact_id":
			claims.ContactID = strings.TrimSpace(val)
		case "device_id":
			claims.DeviceID = strings.TrimSpace(val)
		case "roles":
			claims.Ext[key] = splitCommaTrimmed(val)
		case "partition_ids":
			// Restore multi-partition extras for GetPartitionIDs.
			if ids := splitCommaTrimmed(val); len(ids) > 0 {
				claims.Ext["partition_ids"] = ids
			}
		default:
			// Skip primary values ("sub", "tenant_id", "partition_id")
			if key == "sub" || key == "tenant_id" || key == "partition_id" {
				continue
			}
			// Add other fields to Ext
			claims.Ext[key] = val
		}
	}

	claims.NormalizeIdentity()
	return claims
}

func splitCommaTrimmed(val string) []string {
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// EnrichTenancyClaims internal services act on behalf of different users
// Although they have their claims in place there may be situations where there is need to login as
// This is where secondary claims come into play and implementing systems can decide to use the secondary claims
// This should be done with very high caution though.
func EnrichTenancyClaims(
	ctx context.Context, tenantID, partitionID, accessID string,
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
	secondaryClaims.AccessID = accessID

	return util.SetTenancy(ctx, secondaryClaims)
}
