package frame_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testnats"
	"github.com/pitabwire/frame/frametests/deps/testoryhydra"
	"github.com/pitabwire/frame/frametests/deps/testoryketo"
	"github.com/pitabwire/frame/frametests/deps/testpostgres"
	"github.com/pitabwire/frame/tests"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame"
)

// AuthorizationTestSuite extends FrameBaseTestSuite for comprehensive authorization testing.
type AuthorizationTestSuite struct {
	tests.BaseTestSuite
}

func initResources(_ context.Context) []definition.TestResource {
	pg := testpostgres.NewWithOpts("frame_test_service",
		definition.WithUserName("ant"), definition.WithPassword("s3cr3t"),
		definition.WithEnableLogging(false), definition.WithUseHostMode(false))

	queue := testnats.NewWithOpts("partition",
		definition.WithUserName("ant"),
		definition.WithPassword("s3cr3t"),
		definition.WithEnableLogging(false))

	hydra := testoryhydra.NewWithOpts(
		testoryhydra.HydraConfiguration, definition.WithDependancies(pg),
		definition.WithEnableLogging(false),
		definition.WithUseHostMode(false))

	keto := testoryketo.NewWithOpts(
		testoryketo.KetoConfiguration, definition.WithDependancies(pg),
		definition.WithEnableLogging(true),
		definition.WithUseHostMode(false))

	resources := []definition.TestResource{pg, queue, hydra, keto}
	return resources
}

func (s *AuthorizationTestSuite) SetupSuite() {
	if s.InitResourceFunc == nil {
		s.InitResourceFunc = initResources
	}
	s.BaseTestSuite.SetupSuite()
}

// TestAuthorizationSuite runs the authorization test suite.
func TestAuthorizationSuite(t *testing.T) {
	suite.Run(t, &AuthorizationTestSuite{})
}

// authorizationControlListWrite writes authorization control list entries.
func (s *AuthorizationTestSuite) authorizationControlListWrite(ctx context.Context, writeServerURL string, action string, subject string) error {
	authClaims := frame.ClaimsFromContext(ctx)
	service := frame.Svc(ctx)

	if authClaims == nil {
		return errors.New("only authenticated requests should be used to check authorization")
	}

	payload := map[string]any{
		"namespace":  authClaims.GetTenantID(),
		"object":     authClaims.GetPartitionID(),
		"relation":   action,
		"subject_id": subject,
	}

	status, result, err := service.InvokeRestService(ctx,
		http.MethodPut, writeServerURL, payload, nil)

	if err != nil {
		return err
	}

	if status > 299 || status < 200 {
		return fmt.Errorf("invalid response status %d had message %s", status, string(result))
	}

	var response map[string]any
	err = json.Unmarshal(result, &response)
	if err != nil {
		return err
	}

	return nil
}

// TestAuthorizationControlListWrite tests writing authorization control list entries.
func (s *AuthorizationTestSuite) TestAuthorizationControlListWrite() {
	testCases := []struct {
		name        string
		action      string
		subject     string
		expectError bool
		setupClaims func(context.Context) context.Context
	}{
		{
			name:        "successful authorization write",
			action:      "read",
			subject:     "tested",
			expectError: false,
			setupClaims: func(ctx context.Context) context.Context {
				authClaim := frame.AuthenticationClaims{
					Ext: map[string]any{
						"partition_id": "partition",
						"tenant_id":    "default",
						"access_id":    "access",
					},
				}
				authClaim.Subject = "profile"
				return authClaim.ClaimsToContext(ctx)
			},
		},
		{
			name:        "write with missing claims",
			action:      "read",
			subject:     "tested",
			expectError: true,
			setupClaims: func(ctx context.Context) context.Context {
				return ctx // No claims set
			},
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := t.Context()
				keto := dep.ByImageName(testoryketo.OryKetoImage) // Keto is treated as a queue service

				ketoUri := keto.GetDS(ctx)
				ketoAdminUri := ketoUri.ExtendPath("admin/relation-tuples")

				portMapping, err := keto.PortMapping(ctx, "4466/tcp")
				require.NoError(t, err)

				ketoReadUri := ketoUri.ExtendPath("/relation-tuples/check")
				ketoReadUri, err = ketoReadUri.ChangePort(portMapping)
				require.NoError(t, err)

				// Setup service and context
				ctx, srv := frame.NewService("Test Srv", frame.WithConfig(&frame.ConfigurationDefault{
					AuthorizationServiceWriteURI: ketoAdminUri.String(),
					AuthorizationServiceReadURI:  ketoReadUri.String(),
				}))
				ctx = frame.SvcToContext(ctx, srv)
				ctx = tc.setupClaims(ctx)

				err = s.authorizationControlListWrite(ctx, ketoAdminUri.String(), tc.action, tc.subject)

				if tc.expectError {
					require.Error(t, err, "expected authorization write to fail")
				} else {
					require.NoError(t, err, "authorization write should succeed")
				}
			})
		}
	})
}

// TestAuthHasAccess tests checking access permissions.
func (s *AuthorizationTestSuite) TestAuthHasAccess() {
	testCases := []struct {
		name         string
		action       string
		subject      string
		checkSubject string
		checkAction  string
		expectAccess bool
		expectError  bool
		setupClaims  func(context.Context) context.Context
	}{
		{
			name:         "successful access check with existing permission",
			action:       "read",
			subject:      "reader",
			checkSubject: "reader",
			checkAction:  "read",
			expectAccess: true,
			expectError:  false,
			setupClaims: func(ctx context.Context) context.Context {
				authClaim := frame.AuthenticationClaims{
					Ext: map[string]any{
						"partition_id": "partition",
						"tenant_id":    "default",
						"access_id":    "access",
					},
				}
				authClaim.Subject = "profile"
				return authClaim.ClaimsToContext(ctx)
			},
		},
		{
			name:         "access check with non-existing permission",
			action:       "read",
			subject:      "reader",
			checkSubject: "nonexistent",
			checkAction:  "read",
			expectAccess: false,
			expectError:  true,
			setupClaims: func(ctx context.Context) context.Context {
				authClaim := frame.AuthenticationClaims{
					Ext: map[string]any{
						"partition_id": "partition",
						"tenant_id":    "default",
						"access_id":    "access",
					},
				}
				authClaim.Subject = "profile"
				return authClaim.ClaimsToContext(ctx)
			},
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := t.Context()
				keto := dep.ByImageName(testoryketo.OryKetoImage) // Keto is treated as a queue service

				ketoUri := keto.GetDS(ctx)
				ketoAdminUri := ketoUri.ExtendPath("admin/relation-tuples")
				ketoReadUri := ketoUri.ExtendPath("/relation-tuples/check")

				// Setup service and context
				ctx, srv := frame.NewService("Test Srv", frame.WithConfig(
					&frame.ConfigurationDefault{
						AuthorizationServiceReadURI:  ketoReadUri.String(),
						AuthorizationServiceWriteURI: ketoAdminUri.String(),
					}))
				ctx = frame.SvcToContext(ctx, srv)
				ctx = tc.setupClaims(ctx)

				// First write the permission
				err := s.authorizationControlListWrite(ctx, ketoAdminUri.String(), tc.action, tc.subject)
				if tc.subject == tc.checkSubject { // Only expect success if we're checking the subject we wrote
					require.NoError(t, err, "authorization write should succeed for setup")
				}

				// Then check access
				access, err := frame.AuthHasAccess(ctx, tc.checkAction, tc.checkSubject)

				if tc.expectError {
					require.Error(t, err, "expected access check to fail")
				} else {
					require.NoError(t, err, "access check should succeed")
					if tc.expectAccess {
						require.True(t, access, "expected access to be granted")
					} else {
						require.False(t, access, "expected access to be denied")
					}
				}
			})
		}
	})
}
