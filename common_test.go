package frame_test

import (
	"slices"
	"testing"

	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/tests"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame"
)

type name struct {
	frame.ConfigurationDefault
}

// CommonTestSuite extends FrameBaseTestSuite for comprehensive common functionality testing.
type CommonTestSuite struct {
	tests.BaseTestSuite
}

// TestCommonSuite runs the common test suite.
func TestCommonSuite(t *testing.T) {
	suite.Run(t, &CommonTestSuite{})
}

// TestConfigProcess tests configuration processing from environment variables.
func (s *CommonTestSuite) TestConfigProcess() {
	testCases := []struct {
		name        string
		envVars     map[string]string
		expectError bool
		checkConfig func(*testing.T, name)
	}{
		{
			name: "process PORT and DATABASE_URL environment variables",
			envVars: map[string]string{
				"PORT":         "testingp",
				"DATABASE_URL": "testingu",
			},
			expectError: false,
			checkConfig: func(t *testing.T, conf name) {
				require.Equal(t, "testingp", conf.ServerPort, "PORT environment variable should be processed")
				require.True(t, slices.Contains(conf.GetDatabasePrimaryHostURL(), "testingu"),
					"DATABASE_URL environment variable should be processed")
			},
		},
		{
			name:        "process with missing environment variables",
			envVars:     map[string]string{},
			expectError: false,
			checkConfig: func(t *testing.T, conf name) {
				// Configuration should still be created with defaults
				require.NotNil(t, conf, "configuration should be created even with missing env vars")
			},
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Set environment variables
				for key, value := range tc.envVars {
					t.Setenv(key, value)
				}

				conf, err := frame.ConfigFromEnv[name]()

				if tc.expectError {
					require.Error(t, err, "expected configuration loading to fail")
					return
				}

				require.NoError(t, err, "configuration loading should succeed")
				require.NotNil(t, conf, "configuration should not be nil")

				if tc.checkConfig != nil {
					tc.checkConfig(t, conf)
				}
			})
		}
	})
}

// TestConfigCastingIssues tests configuration casting and service initialization.
func (s *CommonTestSuite) TestConfigCastingIssues() {
	testCases := []struct {
		name        string
		envVars     map[string]string
		expectError bool
		expectCast  bool
	}{
		{
			name: "successful config casting with environment variables",
			envVars: map[string]string{
				"PORT":         "testingp",
				"DATABASE_URL": "testingu",
			},
			expectError: false,
			expectCast:  true,
		},
		{
			name:        "config casting with default values",
			envVars:     map[string]string{},
			expectError: false,
			expectCast:  true,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Set environment variables
				for key, value := range tc.envVars {
					t.Setenv(key, value)
				}

				conf, err := frame.ConfigFromEnv[name]()

				if tc.expectError {
					require.Error(t, err, "expected configuration loading to fail")
					return
				}

				require.NoError(t, err, "configuration loading should succeed")

				// Test service creation and config casting
				_, srv := frame.NewService("Test Srv", frame.WithConfig(&conf))
				require.NotNil(t, srv, "service should be created successfully")

				_, ok := srv.Config().(frame.ConfigurationOAUTH2)
				if tc.expectCast {
					require.True(t, ok, "configuration should be castable to OAUTH2 interface")
				} else {
					require.False(t, ok, "configuration should not be castable to OAUTH2 interface")
				}
			})
		}
	})
}
