package permissions_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/client"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testnats"
	"github.com/pitabwire/frame/frametests/deps/testoryhydra"
	"github.com/pitabwire/frame/frametests/deps/testoryketo"
	"github.com/pitabwire/frame/frametests/deps/testpostgres"
	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/tests"
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
func (s *AuthorizationTestSuite) authorizationControlListWrite(
	ctx context.Context,
	writeServerURL string,
	objectID, action string,
	subject string,
) error {
	authClaims := security.ClaimsFromContext(ctx)

	var opts []client.HTTPOption
	cfg := config.FromContext[config.ConfigurationTraceRequests](ctx)
	if cfg.TraceReq() {
		opts = append(opts, client.WithHTTPTraceRequests(), client.WithHTTPTraceRequestHeaders())
	}
	invoker := client.NewManager(ctx, opts...)

	if authClaims == nil {
		return errors.New("only authenticated requests should be used to check authorization")
	}

	payload := map[string]any{
		"namespace":  authClaims.GetPartitionID(),
		"object":     objectID,
		"relation":   action,
		"subject_id": subject,
	}

	status, result, err := invoker.Invoke(ctx,
		http.MethodPut, writeServerURL, payload, nil)

	if err != nil {
		return err
	}

	if status < http.StatusOK || status >= http.StatusMultipleChoices {
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
		objectID    string
		action      string
		subject     string
		expectError bool
		setupClaims func(context.Context) context.Context
	}{
		{
			name:        "successful authorization write",
			objectID:    "sedf233",
			action:      "read",
			subject:     "tested",
			expectError: false,
			setupClaims: func(ctx context.Context) context.Context {
				authClaim := security.AuthenticationClaims{
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
			objectID:    "sedf234",
			action:      "read",
			subject:     "tested",
			expectError: true,
			setupClaims: func(ctx context.Context) context.Context {
				return ctx // No claims set
			},
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := t.Context()
				keto := dep.ByImageName(testoryketo.OryKetoImage) // Keto is treated as a queue service

				ketoURI := keto.GetDS(ctx)
				ketoAdminURI := ketoURI.ExtendPath("admin/relation-tuples")

				portMapping, err := keto.PortMapping(ctx, "4466/tcp")
				require.NoError(t, err)

				ketoReadURI := ketoURI.ExtendPath("/relation-tuples/check")
				ketoReadURI, err = ketoReadURI.ChangePort(portMapping)
				require.NoError(t, err)

				// Setup service and context
				ctx, srv := frame.NewService(frame.WithName("Test Srv"), frame.WithConfig(&config.ConfigurationDefault{
					AuthorizationServiceWriteURI: ketoAdminURI.String(),
					AuthorizationServiceReadURI:  ketoReadURI.String(),
				}))
				ctx = frame.ToContext(ctx, srv)
				ctx = tc.setupClaims(ctx)

				err = s.authorizationControlListWrite(ctx, ketoAdminURI.String(), tc.objectID, tc.action, tc.subject)

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
		objectID     string
		action       string
		subject      string
		checkSubject string
		checkAction  string
		expectAccess bool
		expectError  bool
		setupClaims  func(context.Context, string) context.Context
	}{
		{
			name:         "successful access check with existing permission",
			objectID:     "sedf433",
			action:       "read",
			subject:      "reader",
			checkSubject: "reader",
			checkAction:  "read",
			expectAccess: true,
			expectError:  false,
			setupClaims: func(ctx context.Context, subject string) context.Context {
				authClaim := security.AuthenticationClaims{
					Ext: map[string]any{
						"partition_id": "partition",
						"tenant_id":    "default",
						"access_id":    "access",
					},
				}
				authClaim.Subject = subject
				return authClaim.ClaimsToContext(ctx)
			},
		},
		{
			name:         "access check with non-existing permission",
			objectID:     "sedf434",
			action:       "read",
			subject:      "reader",
			checkSubject: "nonexistent",
			checkAction:  "read",
			expectAccess: false,
			expectError:  true,
			setupClaims: func(ctx context.Context, subject string) context.Context {
				authClaim := security.AuthenticationClaims{
					Ext: map[string]any{
						"partition_id": "partition",
						"tenant_id":    "default",
						"access_id":    "access",
					},
				}
				authClaim.Subject = subject
				return authClaim.ClaimsToContext(ctx)
			},
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := t.Context()
				keto := dep.ByImageName(testoryketo.OryKetoImage) // Keto is treated as a queue service

				ketoURI := keto.GetDS(ctx)
				ketoAdminURI := ketoURI.ExtendPath("admin/relation-tuples")

				ketoReadURI := ketoURI.ExtendPath("/relation-tuples/check")
				portMapping, err := keto.PortMapping(ctx, "4466/tcp")
				require.NoError(t, err)

				ketoReadURI, err = ketoReadURI.ChangePort(portMapping)
				require.NoError(t, err)

				// Setup service and context
				ctx, srv := frame.NewService(frame.WithName("Test Srv"), frame.WithConfig(
					&config.ConfigurationDefault{
						AuthorizationServiceReadURI:  ketoReadURI.String(),
						AuthorizationServiceWriteURI: ketoAdminURI.String(),
					}))

				ctx = tc.setupClaims(ctx, tc.checkSubject)

				sm := srv.SecurityManager()
				// First write the permission
				err = s.authorizationControlListWrite(ctx, ketoAdminURI.String(), tc.objectID, tc.action, tc.subject)
				if tc.subject == tc.checkSubject { // Only expect success if we're checking the subject we wrote
					require.NoError(t, err, "authorization write should succeed for setup")
				}

				authorizer := sm.GetAuthorizer(ctx)
				// Then check access
				access, err := authorizer.HasAccess(ctx, tc.objectID, tc.checkAction)

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
