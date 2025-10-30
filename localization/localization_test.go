package localization_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/metadata"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/localization"
	lgrpc "github.com/pitabwire/frame/localization/interceptors/grpc"
	lhttp "github.com/pitabwire/frame/localization/interceptors/http"
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
			serviceName:    "Test LocalizationManager Srv",
			translationDir: "test_data",
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
		s.Run(tc.name, func() {
			lm := localization.NewManager(tc.translationDir, tc.languages...)

			bundle := lm.Bundle()

			enLocalizer := i18n.NewLocalizer(bundle, "en", "sw")
			englishVersion, err := enLocalizer.Localize(&i18n.LocalizeConfig{
				DefaultMessage: &i18n.Message{
					ID: tc.messageID,
				},
				TemplateData: tc.templateData,
				PluralCount:  tc.pluralCount,
			})
			s.Require().NoError(err, "English localization should succeed")
			s.Require().Equal(tc.expectedEn, englishVersion, "English translation should match expected")

			swLocalizer := i18n.NewLocalizer(bundle, "sw")
			swVersion, err := swLocalizer.Localize(&i18n.LocalizeConfig{
				DefaultMessage: &i18n.Message{
					ID: tc.messageID,
				},
				TemplateData: tc.templateData,
				PluralCount:  tc.pluralCount,
			})
			s.Require().NoError(err, "Swahili localization should succeed")
			s.Require().Equal(tc.expectedSw, swVersion, "Swahili translation should match expected")
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
			serviceName:    "Test LocalizationManager Srv",
			translationDir: "test_data",
			languages:      []string{"en", "sw"},
			messageID:      "Example",
			language:       "en",
			templateData:   nil,
			pluralCount:    1,
			expected:       "<no value> has nothing",
		},
		{
			name:           "translate with template data",
			serviceName:    "Test LocalizationManager Srv",
			translationDir: "test_data",
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
			serviceName:    "Test LocalizationManager Srv",
			translationDir: "test_data",
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
		s.Run(tc.name, func() {
			ctx := context.Background()
			lm := localization.NewManager(tc.translationDir, tc.languages...)

			var result string
			switch {
			case tc.templateData == nil:
				result = lm.Translate(ctx, tc.language, tc.messageID)
			case tc.pluralCount > 1:
				result = lm.TranslateWithMapAndCount(ctx, tc.language, tc.messageID, tc.templateData, tc.pluralCount)
			default:
				result = lm.TranslateWithMap(ctx, tc.language, tc.messageID, tc.templateData)
			}

			s.Require().Equal(tc.expected, result, "Translation result should match expected")
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
			serviceName: "Test LocalizationManager Srv",
			language:    "en",
			messageID:   "Example",
			expected:    "<no value> has nothing",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			ctx := context.Background()
			lm := localization.NewManager("test_data", "en", "sw")

			ctx = localization.ToContext(ctx, []string{tc.language})
			result := lm.Translate(ctx, "", tc.messageID)
			s.Require().Equal(tc.expected, result, "Translation with language context should match expected")

			lang := localization.FromContext(ctx)
			s.Require().Equal([]string{tc.language}, lang, "Language from context should match set language")
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
			serviceName: "Test LocalizationManager Srv",
			anyMap: map[string]string{
				"world": "data",
			},
			testLanguages: []string{"en", "sw"},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			_, _ = frame.NewService(tc.serviceName)

			// Test language map functions
			testMap := localization.ToMap(tc.anyMap, tc.testLanguages)

			result := localization.FromMap(testMap)
			s.Require().Equal(result, tc.testLanguages, "Language map should contain test key")
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
			serviceName:  "Test LocalizationManager Srv",
			requestPath:  "/test",
			acceptLang:   "en-US,en;q=0.9",
			expectedLang: "en",
		},
		{
			name:         "HTTP middleware with swahili accept-language",
			serviceName:  "Test LocalizationManager Srv",
			requestPath:  "/test",
			acceptLang:   "sw",
			expectedLang: "sw",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// lm := localization.NewManager("test_data", "en", "sw")

			middleware := lhttp.LanguageHTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				lang := localization.FromContext(r.Context())
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(strings.Join(lang, ",")))
			}))

			req := httptest.NewRequest(http.MethodGet, tc.requestPath, nil)
			req.Header.Set("Accept-Language", tc.acceptLang)

			w := httptest.NewRecorder()
			middleware.ServeHTTP(w, req)

			result := w.Body.String()
			s.Require().Contains(result, tc.expectedLang, "HTTP response should contain expected language")
		})
	}
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
			serviceName:  "Test LocalizationManager Srv",
			metadataLang: "en",
			expectedLang: []string{"en"},
		},
		{
			name:         "gRPC unary interceptor with swahili metadata",
			serviceName:  "Test LocalizationManager Srv",
			metadataLang: "sw",
			expectedLang: []string{"sw"},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// lm := localization.NewManager("test_data", "en", "sw")

			interceptor := lgrpc.LanguageUnaryInterceptor()
			handler := func(ctx context.Context, _ any) (any, error) {
				lang := localization.FromContext(ctx)
				return strings.Join(lang, ","), nil
			}

			md := metadata.New(map[string]string{"accept-language": tc.metadataLang})
			ctx := metadata.NewIncomingContext(context.Background(), md)

			result, err := interceptor(ctx, nil, nil, handler)
			s.Require().NoError(err, "gRPC interceptor should succeed")
			s.Require().Contains(result.(string), tc.expectedLang[0], "gRPC interceptor should detect correct language")
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
			serviceName:  "Test LocalizationManager Srv",
			metadataLang: "en",
			expectedLang: []string{"en"},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			ctx, _ := frame.NewService(tc.serviceName)

			md := metadata.New(map[string]string{"accept-language": tc.metadataLang})
			grpcCtx := metadata.NewIncomingContext(ctx, md)

			lang := localization.ExtractLanguageFromGrpcRequest(grpcCtx)
			s.Require().Equal(tc.expectedLang, lang, "Language from gRPC request should match expected")
		})
	}
}
