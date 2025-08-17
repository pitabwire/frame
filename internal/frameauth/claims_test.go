package frameauth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
)

// FrameAuthClaimsTestSuite extends the base test suite for claims testing
type FrameAuthClaimsTestSuite struct {
	suite.Suite
}

func (suite *FrameAuthClaimsTestSuite) TestAuthenticationClaimsGetters() {
	tests := []struct {
		name        string
		claims      *AuthenticationClaims
		expected    map[string]string
		description string
	}{
		{
			name: "DirectFieldValues",
			claims: &AuthenticationClaims{
				TenantID:    "direct-tenant",
				PartitionID: "direct-partition",
				AccessID:    "direct-access",
				ContactID:   "direct-contact",
				SessionID:   "direct-session",
				DeviceID:    "direct-device",
				ServiceName: "direct-service",
				Roles:       []string{"admin", "user"},
			},
			expected: map[string]string{
				"tenant_id":    "direct-tenant",
				"partition_id": "direct-partition",
				"access_id":    "direct-access",
				"contact_id":   "direct-contact",
				"session_id":   "direct-session",
				"device_id":    "direct-device",
				"service_name": "direct-service",
			},
			description: "Should return direct field values when available",
		},
		{
			name: "ExtFieldValues",
			claims: &AuthenticationClaims{
				Ext: map[string]any{
					"tenant_id":    "ext-tenant",
					"partition_id": "ext-partition",
					"access_id":    "ext-access",
					"contact_id":   "ext-contact",
					"session_id":   "ext-session",
					"device_id":    "ext-device",
					"service_name": "ext-service",
					"roles":        "admin,user",
				},
			},
			expected: map[string]string{
				"tenant_id":    "ext-tenant",
				"partition_id": "ext-partition",
				"access_id":    "ext-access",
				"contact_id":   "ext-contact",
				"session_id":   "ext-session",
				"device_id":    "ext-device",
				"service_name": "ext-service",
			},
			description: "Should return ext field values when direct fields are empty",
		},
		{
			name: "DirectOverridesExt",
			claims: &AuthenticationClaims{
				TenantID: "direct-tenant",
				Ext: map[string]any{
					"tenant_id": "ext-tenant",
				},
			},
			expected: map[string]string{
				"tenant_id": "direct-tenant",
			},
			description: "Should prioritize direct fields over ext fields",
		},
		{
			name: "EmptyValues",
			claims: &AuthenticationClaims{
				Ext: map[string]any{},
			},
			expected: map[string]string{
				"tenant_id":    "",
				"partition_id": "",
				"access_id":    "",
				"contact_id":   "",
				"session_id":   "",
				"device_id":    "",
				"service_name": "",
			},
			description: "Should return empty strings when no values are set",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.Equal(tt.expected["tenant_id"], tt.claims.GetTenantID(), tt.description)
			suite.Equal(tt.expected["partition_id"], tt.claims.GetPartitionID(), tt.description)
			suite.Equal(tt.expected["access_id"], tt.claims.GetAccessID(), tt.description)
			suite.Equal(tt.expected["contact_id"], tt.claims.GetContactID(), tt.description)
			suite.Equal(tt.expected["session_id"], tt.claims.GetSessionID(), tt.description)
			suite.Equal(tt.expected["device_id"], tt.claims.GetDeviceID(), tt.description)
			suite.Equal(tt.expected["service_name"], tt.claims.GetServiceName(), tt.description)
		})
	}
}

func (suite *FrameAuthClaimsTestSuite) TestGetRoles() {
	tests := []struct {
		name        string
		claims      *AuthenticationClaims
		expected    []string
		description string
	}{
		{
			name: "DirectRoles",
			claims: &AuthenticationClaims{
				Roles: []string{"admin", "user"},
			},
			expected:    []string{"admin", "user"},
			description: "Should return direct roles when available",
		},
		{
			name: "ExtRolesString",
			claims: &AuthenticationClaims{
				Ext: map[string]any{
					"roles": "admin,user,guest",
				},
			},
			expected:    []string{"admin", "user", "guest"},
			description: "Should parse comma-separated roles from ext field",
		},
		{
			name: "ExtRoleString",
			claims: &AuthenticationClaims{
				Ext: map[string]any{
					"role": "admin,user",
				},
			},
			expected:    []string{"admin", "user"},
			description: "Should parse comma-separated roles from ext 'role' field",
		},
		{
			name: "EmptyRoles",
			claims: &AuthenticationClaims{
				Ext: map[string]any{},
			},
			expected:    []string{},
			description: "Should return empty slice when no roles are set",
		},
		{
			name: "DirectOverridesExt",
			claims: &AuthenticationClaims{
				Roles: []string{"direct-admin"},
				Ext: map[string]any{
					"roles": "ext-user",
				},
			},
			expected:    []string{"direct-admin"},
			description: "Should prioritize direct roles over ext roles",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			result := tt.claims.GetRoles()
			suite.Equal(tt.expected, result, tt.description)
		})
	}
}

func (suite *FrameAuthClaimsTestSuite) TestIsInternalSystem() {
	tests := []struct {
		name        string
		claims      *AuthenticationClaims
		expected    bool
		description string
	}{
		{
			name: "InternalSystemRole",
			claims: &AuthenticationClaims{
				Roles: []string{"system_internal_service"},
			},
			expected:    true,
			description: "Should return true for system_internal role",
		},
		{
			name: "MultipleRoles",
			claims: &AuthenticationClaims{
				Roles: []string{"user", "admin"},
			},
			expected:    false,
			description: "Should return false for multiple roles",
		},
		{
			name: "NonInternalRole",
			claims: &AuthenticationClaims{
				Roles: []string{"user"},
			},
			expected:    false,
			description: "Should return false for non-internal role",
		},
		{
			name: "NoRoles",
			claims: &AuthenticationClaims{
				Roles: []string{},
			},
			expected:    false,
			description: "Should return false when no roles are set",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			result := tt.claims.isInternalSystem()
			suite.Equal(tt.expected, result, tt.description)
		})
	}
}

func (suite *FrameAuthClaimsTestSuite) TestAsMetadata() {
	tests := []struct {
		name        string
		claims      *AuthenticationClaims
		expected    map[string]string
		description string
	}{
		{
			name: "CompleteMetadata",
			claims: &AuthenticationClaims{
				TenantID:    "test-tenant",
				PartitionID: "test-partition",
				AccessID:    "test-access",
				ContactID:   "test-contact",
				DeviceID:    "test-device",
				Roles:       []string{"admin", "user"},
			},
			expected: map[string]string{
				"sub":          "",
				"tenant_id":    "test-tenant",
				"partition_id": "test-partition",
				"access_id":    "test-access",
				"contact_id":   "test-contact",
				"device_id":    "test-device",
				"roles":        "admin,user",
			},
			description: "Should create complete metadata map",
		},
		{
			name: "EmptyMetadata",
			claims: &AuthenticationClaims{
				Ext: map[string]any{},
			},
			expected: map[string]string{
				"sub":          "",
				"tenant_id":    "",
				"partition_id": "",
				"access_id":    "",
				"contact_id":   "",
				"device_id":    "",
				"roles":        "",
			},
			description: "Should create metadata map with empty values",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			result := tt.claims.AsMetadata()
			suite.Equal(tt.expected, result, tt.description)
		})
	}
}

func (suite *FrameAuthClaimsTestSuite) TestClaimsToContext() {
	tests := []struct {
		name        string
		claims      *AuthenticationClaims
		expectSkip  bool
		description string
	}{
		{
			name: "RegularClaims",
			claims: &AuthenticationClaims{
				TenantID: "test-tenant",
				Roles:    []string{"user"},
			},
			expectSkip:  false,
			description: "Should not skip tenancy checks for regular claims",
		},
		{
			name: "InternalSystemClaims",
			claims: &AuthenticationClaims{
				TenantID: "test-tenant",
				Roles:    []string{"system_internal_service"},
			},
			expectSkip:  true,
			description: "Should skip tenancy checks for internal system claims",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			ctx := context.Background()
			resultCtx := tt.claims.ClaimsToContext(ctx)

			// Check that claims are in context
			retrievedClaims := ClaimsFromContext(resultCtx)
			suite.Equal(tt.claims, retrievedClaims, tt.description)

			// Check tenancy skip flag
			skipTenancy := IsTenancyChecksOnClaimSkipped(resultCtx)
			suite.Equal(tt.expectSkip, skipTenancy, tt.description)
		})
	}
}

func (suite *FrameAuthClaimsTestSuite) TestClaimsFromMap() {
	tests := []struct {
		name        string
		input       map[string]string
		expected    *AuthenticationClaims
		description string
	}{
		{
			name: "CompleteMap",
			input: map[string]string{
				"sub":          "test-subject",
				"tenant_id":    "test-tenant",
				"partition_id": "test-partition",
				"access_id":    "test-access",
				"contact_id":   "test-contact",
				"device_id":    "test-device",
				"roles":        "admin,user",
				"custom_field": "custom_value",
			},
			expected: &AuthenticationClaims{
				TenantID:    "test-tenant",
				PartitionID: "test-partition",
				AccessID:    "test-access",
				ContactID:   "test-contact",
				DeviceID:    "test-device",
				Ext: map[string]any{
					"roles":        []string{"admin", "user"},
					"custom_field": "custom_value",
				},
			},
			description: "Should create claims from complete map",
		},
		{
			name: "EmptyMap",
			input: map[string]string{},
			expected: nil,
			description: "Should return nil for empty map",
		},
		{
			name: "OnlySubject",
			input: map[string]string{
				"sub": "test-subject",
			},
			expected: &AuthenticationClaims{
				TenantID:    "",
				PartitionID: "",
				Ext:         map[string]any{},
			},
			description: "Should create claims with only subject",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			result := ClaimsFromMap(tt.input)
			if tt.expected == nil {
				suite.Nil(result, tt.description)
			} else {
				suite.NotNil(result, tt.description)
				suite.Equal(tt.expected.TenantID, result.TenantID, tt.description)
				suite.Equal(tt.expected.PartitionID, result.PartitionID, tt.description)
				suite.Equal(tt.expected.AccessID, result.AccessID, tt.description)
				suite.Equal(tt.expected.ContactID, result.ContactID, tt.description)
				suite.Equal(tt.expected.DeviceID, result.DeviceID, tt.description)
				
				if tt.input["sub"] != "" {
					suite.Equal(tt.input["sub"], result.Subject, tt.description)
				}
			}
		})
	}
}

func (suite *FrameAuthClaimsTestSuite) TestContextOperations() {
	suite.Run("JwtToFromContext", func() {
		ctx := context.Background()
		testJWT := "test-jwt-token"

		// Add JWT to context
		ctxWithJWT := jwtToContext(ctx, testJWT)

		// Retrieve JWT from context
		retrievedJWT := JwtFromContext(ctxWithJWT)
		suite.Equal(testJWT, retrievedJWT, "Should retrieve the same JWT from context")

		// Test with empty context
		emptyJWT := JwtFromContext(ctx)
		suite.Equal("", emptyJWT, "Should return empty string for context without JWT")
	})

	suite.Run("ClaimsFromContext", func() {
		ctx := context.Background()
		claims := &AuthenticationClaims{
			TenantID: "test-tenant",
		}

		// Add claims to context
		ctxWithClaims := claims.ClaimsToContext(ctx)

		// Retrieve claims from context
		retrievedClaims := ClaimsFromContext(ctxWithClaims)
		suite.Equal(claims, retrievedClaims, "Should retrieve the same claims from context")

		// Test with empty context
		emptyClaims := ClaimsFromContext(ctx)
		suite.Nil(emptyClaims, "Should return nil for context without claims")
	})

	suite.Run("TenancySkipOperations", func() {
		ctx := context.Background()

		// Initially should not be skipped
		suite.False(IsTenancyChecksOnClaimSkipped(ctx), "Should not skip tenancy checks initially")

		// Skip tenancy checks
		ctxWithSkip := SkipTenancyChecksOnClaims(ctx)
		suite.True(IsTenancyChecksOnClaimSkipped(ctxWithSkip), "Should skip tenancy checks after setting")

		// Original context should remain unchanged
		suite.False(IsTenancyChecksOnClaimSkipped(ctx), "Original context should remain unchanged")
	})
}

func (suite *FrameAuthClaimsTestSuite) TestEdgeCases() {
	suite.Run("NilExtMap", func() {
		claims := &AuthenticationClaims{
			Ext: nil,
		}

		suite.NotPanics(func() {
			_ = claims.GetTenantID()
			_ = claims.GetPartitionID()
			_ = claims.GetAccessID()
			_ = claims.GetContactID()
			_ = claims.GetSessionID()
			_ = claims.GetDeviceID()
			_ = claims.GetServiceName()
			_ = claims.GetRoles()
		}, "Should not panic with nil Ext map")
	})

	suite.Run("InvalidTypeInExt", func() {
		claims := &AuthenticationClaims{
			Ext: map[string]any{
				"tenant_id": 123, // Invalid type
				"roles":     456, // Invalid type
			},
		}

		suite.NotPanics(func() {
			tenantID := claims.GetTenantID()
			suite.Equal("", tenantID, "Should return empty string for invalid type")

			roles := claims.GetRoles()
			suite.Equal([]string{}, roles, "Should return empty slice for invalid type")
		}, "Should handle invalid types gracefully")
	})
}

func (suite *FrameAuthClaimsTestSuite) TestConcurrentAccess() {
	const numGoroutines = 10
	const numOperations = 100

	suite.Run("ConcurrentClaimsAccess", func() {
		claims := &AuthenticationClaims{
			TenantID:    "test-tenant",
			PartitionID: "test-partition",
			Roles:       []string{"admin", "user"},
			Ext: map[string]any{
				"custom_field": "custom_value",
			},
		}

		done := make(chan bool, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer func() { done <- true }()

				for j := 0; j < numOperations; j++ {
					suite.NotPanics(func() {
						_ = claims.GetTenantID()
						_ = claims.GetPartitionID()
						_ = claims.GetRoles()
						_ = claims.AsMetadata()
					}, "Concurrent access should not panic")
				}
			}()
		}

		for i := 0; i < numGoroutines; i++ {
			<-done
		}
	})
}

func TestFrameAuthClaimsTestSuite(t *testing.T) {
	suite.Run(t, new(FrameAuthClaimsTestSuite))
}
