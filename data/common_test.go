package data_test

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/tests"
)

type name struct {
	config.ConfigurationDefault
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

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Set environment variables
				for key, value := range tc.envVars {
					t.Setenv(key, value)
				}

				conf, err := config.FromEnv[name]()

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

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Set environment variables
				for key, value := range tc.envVars {
					t.Setenv(key, value)
				}

				conf, err := config.FromEnv[name]()

				if tc.expectError {
					require.Error(t, err, "expected configuration loading to fail")
					return
				}

				require.NoError(t, err, "configuration loading should succeed")

				// Test service creation and config casting
				_, srv := frame.NewService("Test Srv", frame.WithConfig(&conf))
				require.NotNil(t, srv, "service should be created successfully")

				_, ok := srv.Config().(config.ConfigurationOAUTH2)
				if tc.expectCast {
					require.True(t, ok, "configuration should be castable to OAUTH2 interface")
				} else {
					require.False(t, ok, "configuration should not be castable to OAUTH2 interface")
				}
			})
		}
	})
}

// TestErrIsNotFound tests the ErrIsNotFound function with various error types.
func (s *CommonTestSuite) TestErrIsNotFound() {
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "gorm.ErrRecordNotFound",
			err:      gorm.ErrRecordNotFound,
			expected: true,
		},
		{
			name:     "sql.ErrNoRows",
			err:      sql.ErrNoRows,
			expected: true,
		},
		{
			name:     "wrapped gorm.ErrRecordNotFound",
			err:      fmt.Errorf("failed to find record: %w", gorm.ErrRecordNotFound),
			expected: true,
		},
		{
			name:     "wrapped sql.ErrNoRows",
			err:      fmt.Errorf("query failed: %w", sql.ErrNoRows),
			expected: true,
		},
		{
			name:     "gRPC NotFound status",
			err:      status.Error(codes.NotFound, "resource not found"),
			expected: true,
		},
		{
			name:     "gRPC other status code",
			err:      status.Error(codes.Internal, "internal error"),
			expected: false,
		},
		{
			name:     "error message with 'not found' lowercase",
			err:      errors.New("user not found"),
			expected: true,
		},
		{
			name:     "error message with 'Not Found' mixed case",
			err:      errors.New("Resource Not Found"),
			expected: true,
		},
		{
			name:     "error message with 'NOT FOUND' uppercase",
			err:      errors.New("ITEM NOT FOUND"),
			expected: true,
		},
		{
			name:     "error message with 'notfound' no space",
			err:      errors.New("itemnotfound"),
			expected: true,
		},
		{
			name:     "error message with '404'",
			err:      errors.New("HTTP 404 error occurred"),
			expected: true,
		},
		{
			name:     "generic error without 'not found'",
			err:      errors.New("something went wrong"),
			expected: false,
		},
		{
			name:     "validation error",
			err:      errors.New("invalid input"),
			expected: false,
		},
		{
			name:     "multiple wrapped errors with gorm.ErrRecordNotFound",
			err:      fmt.Errorf("operation failed: %w", fmt.Errorf("database error: %w", gorm.ErrRecordNotFound)),
			expected: true,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependancyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := frame.ErrIsNotFound(tc.err)
				require.Equal(t, tc.expected, result,
					"ErrIsNotFound(%v) = %v, expected %v", tc.err, result, tc.expected)
			})
		}
	})
}
