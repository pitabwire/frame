package frame_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/tests"
)

// LocalizationTestSuite extends BaseTestSuite for comprehensive localization testing.
type LocalizationTestSuite struct {
	tests.BaseTestSuite
}

// TestLocalizationSuite runs the localization test suite.
func TestLocalizationSuite(t *testing.T) {
	suite.Run(t, &LocalizationTestSuite{})
}

// TestTranslations tests basic translation functionality.
func (s *LocalizationTestSuite) TestTranslations() {
	testCases := []struct {
		name           string
		serviceName    string
		translationDir string
		languages      []string
		messageID      string
		templateData   map[string]any
		pluralCount    int
		expectedEn     string
		expectedSw     string
	}{
		{
			name:           "basic translation with template data",
			serviceName:    "Test Localization Srv",
			translationDir: "tests_runner/localization",
			languages:      []string{"en", "sw"},
			messageID:      "Example",
			templateData: map[string]any{
				"Name": "Air",
			},
			pluralCount: 1,
			expectedEn:  "Air has nothing",
			expectedSw:  "Air haina chochote",
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			translations := frame.WithTranslations(tc.translationDir, tc.languages...)
			_, srv := frame.NewService(tc.serviceName, translations)

			bundle := srv.Bundle()

			enLocalizer := i18n.NewLocalizer(bundle, "en", "sw")
			englishVersion, err := enLocalizer.Localize(&i18n.LocalizeConfig{
				DefaultMessage: &i18n.Message{
					ID: tc.messageID,
				},
				TemplateData: tc.templateData,
				PluralCount:  tc.pluralCount,
			})
			require.NoError(t, err, "English localization should succeed")
			require.Equal(t, tc.expectedEn, englishVersion, "English translation should match expected")

			swLocalizer := i18n.NewLocalizer(bundle, "sw")
			swVersion, err := swLocalizer.Localize(&i18n.LocalizeConfig{
				DefaultMessage: &i18n.Message{
					ID: tc.messageID,
				},
				TemplateData: tc.templateData,
				PluralCount:  tc.pluralCount,
			})
			require.NoError(t, err, "Swahili localization should succeed")
			require.Equal(t, tc.expectedSw, swVersion, "Swahili translation should match expected")
		})
	}
}

// TestTranslationsHelpers tests translation helper methods.
func (s *LocalizationTestSuite) TestTranslationsHelpers() {
	testCases := []struct {
		name           string
		serviceName    string
		translationDir string
		languages      []string
		messageID      string
		language       string
		templateData   map[string]any
		pluralCount    int
		expected       string
	}{
		{
			name:           "translate without template data",
			serviceName:    "Test Localization Srv",
			translationDir: "tests_runner/localization",
			languages:      []string{"en", "sw"},
			messageID:      "Example",
			language:       "en",
			templateData:   nil,
			pluralCount:    1,
			expected:       "<no value> has nothing",
		},
		{
			name:           "translate with template data",
			serviceName:    "Test Localization Srv",
			translationDir: "tests_runner/localization",
			languages:      []string{"en", "sw"},
			messageID:      "Example",
			language:       "en",
			templateData: map[string]any{
				"Name": "MapMan",
			},
			pluralCount: 1,
			expected:    "MapMan has nothing",
		},
		{
			name:           "translate with template data and plural",
			serviceName:    "Test Localization Srv",
			translationDir: "tests_runner/localization",
			languages:      []string{"en", "sw"},
			messageID:      "Example",
			language:       "en",
			templateData: map[string]any{
				"Name": "CountMen",
			},
			pluralCount: 2,
			expected:    "CountMen have nothing",
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			translations := frame.WithTranslations(tc.translationDir, tc.languages...)
			ctx, srv := frame.NewService(tc.serviceName, translations)

			var result string
			if tc.templateData == nil {
				result = srv.Translate(ctx, tc.language, tc.messageID)
			} else if tc.pluralCount > 1 {
				result = srv.TranslateWithMapAndCount(ctx, tc.language, tc.messageID, tc.templateData, tc.pluralCount)
			} else {
				result = srv.TranslateWithMap(ctx, tc.language, tc.messageID, tc.templateData)
			}

			require.Equal(t, tc.expected, result, "Translation result should match expected")
		})
	}
}

// TestLanguageContextManagement tests language context management.
func (s *LocalizationTestSuite) TestLanguageContextManagement() {
	testCases := []struct {
		name        string
		serviceName string
		language    string
		messageID   string
		expected    string
	}{
		{
			name:        "language context management",
			serviceName: "Test Localization Srv",
			language:    "en",
			messageID:   "Example",
			expected:    "<no value> has nothing",
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			translations := frame.WithTranslations("tests_runner/localization", "en", "sw")
			ctx, srv := frame.NewService(tc.serviceName, translations)

			ctx = frame.LangugageToContext(ctx, []string{tc.language})
			result := srv.Translate(ctx, "", tc.messageID)
			require.Equal(t, tc.expected, result, "Translation with language context should match expected")

			lang := frame.LanguageFromContext(ctx)
			require.Equal(t, []string{tc.language}, lang, "Language from context should match set language")
		})
	}
}

// TestLanguageMapManagement tests language map management.
func (s *LocalizationTestSuite) TestLanguageMapManagement() {
	testCases := []struct {
		name          string
		serviceName   string
		anyMap        map[string]string
		testLanguages []string
	}{
		{
			name:        "language map management",
			serviceName: "Test Localization Srv",
			anyMap: map[string]string{
				"world": "data",
			},
			testLanguages: []string{"en", "sw"},
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			_, _ = frame.NewService(tc.serviceName)

			// Test language map functions
			testMap := frame.LanguageToMap(tc.anyMap, tc.testLanguages)

			result := frame.LanguageFromMap(testMap)
			require.Equal(t, result, tc.testLanguages, "Language map should contain test key")
		})
	}
}

// TestLanguageHTTPMiddleware tests HTTP middleware for language detection.
func (s *LocalizationTestSuite) TestLanguageHTTPMiddleware() {
	testCases := []struct {
		name         string
		serviceName  string
		requestPath  string
		acceptLang   string
		expectedLang string
	}{
		{
			name:         "HTTP middleware with accept-language header",
			serviceName:  "Test Localization Srv",
			requestPath:  "/test",
			acceptLang:   "en-US,en;q=0.9",
			expectedLang: "en",
		},
		{
			name:         "HTTP middleware with swahili accept-language",
			serviceName:  "Test Localization Srv",
			requestPath:  "/test",
			acceptLang:   "sw",
			expectedLang: "sw",
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			translations := frame.WithTranslations("tests_runner/localization", "en", "sw")
			_, srv := frame.NewService(tc.serviceName, translations)

			middleware := srv.LanguageHTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				lang := frame.LanguageFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(strings.Join(lang, ",")))
			}))

			req := httptest.NewRequest("GET", tc.requestPath, nil)
			req.Header.Set("Accept-Language", tc.acceptLang)

			w := httptest.NewRecorder()
			middleware.ServeHTTP(w, req)

			result := w.Body.String()
			require.Contains(t, result, tc.expectedLang, "HTTP response should contain expected language")
		})
	}
}

// Mock ServerStream for testing stream interceptors.
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context {
	return m.ctx
}

// TestLanguageGrpcInterceptors tests gRPC interceptors for language detection.
func (s *LocalizationTestSuite) TestLanguageGrpcInterceptors() {
	testCases := []struct {
		name         string
		serviceName  string
		metadataLang string
		expectedLang []string
	}{
		{
			name:         "gRPC unary interceptor with language metadata",
			serviceName:  "Test Localization Srv",
			metadataLang: "en",
			expectedLang: []string{"en"},
		},
		{
			name:         "gRPC unary interceptor with swahili metadata",
			serviceName:  "Test Localization Srv",
			metadataLang: "sw",
			expectedLang: []string{"sw"},
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			translations := frame.WithTranslations("tests_runner/localization", "en", "sw")
			_, srv := frame.NewService(tc.serviceName, translations)

			interceptor := srv.LanguageUnaryInterceptor()
			handler := func(ctx context.Context, req any) (any, error) {
				lang := frame.LanguageFromContext(ctx)
				return strings.Join(lang, ","), nil
			}

			md := metadata.New(map[string]string{"accept-language": tc.metadataLang})
			ctx := metadata.NewIncomingContext(context.Background(), md)

			result, err := interceptor(ctx, nil, nil, handler)
			require.NoError(t, err, "gRPC interceptor should succeed")
			require.Contains(t, result.(string), tc.expectedLang[0], "gRPC interceptor should detect correct language")
		})
	}
}

// TestLanguageFromGrpcRequest tests language extraction from gRPC requests.
func (s *LocalizationTestSuite) TestLanguageFromGrpcRequest() {
	testCases := []struct {
		name         string
		serviceName  string
		metadataLang string
		expectedLang []string
	}{
		{
			name:         "extract language from gRPC request metadata",
			serviceName:  "Test Localization Srv",
			metadataLang: "en",
			expectedLang: []string{"en"},
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			ctx, _ := frame.NewService(tc.serviceName)

			md := metadata.New(map[string]string{"accept-language": tc.metadataLang})
			grpcCtx := metadata.NewIncomingContext(ctx, md)

			lang := frame.LanguageFromGrpcRequest(grpcCtx)
			require.Equal(t, tc.expectedLang, lang, "Language from gRPC request should match expected")
		})
	}
}

// Mock message for testing queue language propagation.
type mockMessage struct {
	metadata map[string]string
	body     []byte
}

func (m *mockMessage) Metadata() map[string]string {
	return m.metadata
}

func (m *mockMessage) Body() []byte {
	return m.body
}

func (m *mockMessage) SetMetadata(md map[string]string) {
	m.metadata = md
}
