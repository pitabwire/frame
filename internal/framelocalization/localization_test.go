package framelocalization_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/pitabwire/frame"
)

func TestTranslations(t *testing.T) {
	translations := frame.WithTranslations("tests_runner/localization", "en", "sw")
	_, srv := frame.NewService("Test Localization Srv", translations)

	bundle := srv.Bundle()

	enLocalizer := i18n.NewLocalizer(bundle, "en", "sw")
	englishVersion, err := enLocalizer.Localize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID: "Example",
		},
		TemplateData: map[string]any{
			"Name": "Air",
		},
		PluralCount: 1,
	})
	if err != nil {
		t.Errorf(" There was an error parsing the translations %s", err)
		return
	}

	if englishVersion != "Air has nothing" {
		t.Errorf(
			"Localizations didn't quite work like they should, found : %s expected : %s",
			englishVersion,
			"Air has nothing",
		)
		return
	}

	swLocalizer := i18n.NewLocalizer(bundle, "sw")
	swVersion, err := swLocalizer.Localize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID: "Example",
		},
		TemplateData: map[string]any{
			"Name": "Air",
		},
		PluralCount: 1,
	})
	if err != nil {
		t.Errorf(" There was an error parsing the translations %s", err)
		return
	}

	if swVersion != "Air haina chochote" {
		t.Errorf(
			"Localizations didn't quite work like they should, found : %s expected : %s",
			swVersion,
			"Air haina chochote",
		)
		return
	}
}

func TestTranslationsHelpers(t *testing.T) {
	translations := frame.WithTranslations("tests_runner/localization", "en", "sw")
	ctx, srv := frame.NewService("Test Localization Srv", translations)

	englishVersion := srv.Translate(ctx, "en", "Example")
	if englishVersion != "<no value> has nothing" {
		t.Errorf(
			"Localizations didn't quite work like they should, found : %s expected : %s",
			englishVersion,
			"<no value> has nothing",
		)
		return
	}

	englishVersion = srv.TranslateWithMap(ctx, "en", "Example", map[string]any{"Name": "MapMan"})
	if englishVersion != "MapMan has nothing" {
		t.Errorf(
			"Localizations didn't quite work like they should, found : %s expected : %s",
			englishVersion,
			"MapMan has nothing",
		)
		return
	}

	englishVersion = srv.TranslateWithMapAndCount(ctx, "en", "Example", map[string]any{"Name": "CountMen"}, 2)
	if englishVersion != "CountMen have nothing" {
		t.Errorf(
			"Localizations didn't quite work like they should, found : %s expected : %s",
			englishVersion,
			"CountMen have nothing",
		)
		return
	}
}

func TestLanguageContextManagement(t *testing.T) {
	// Test adding language to context
	ctx := t.Context()
	languages := []string{"en-US", "fr-FR"}

	ctxWithLang := frame.LangugageToContext(ctx, languages)

	// Test extracting language from context
	extractedLangs := frame.LanguageFromContext(ctxWithLang)

	if !reflect.DeepEqual(languages, extractedLangs) {
		t.Errorf("LanguageFromContext did not return the expected languages: got %v, want %v",
			extractedLangs, languages)
	}

	// Test with empty context
	emptyLangs := frame.LanguageFromContext(ctx)
	if emptyLangs != nil {
		t.Errorf("LanguageFromContext on empty context should return nil, got %v", emptyLangs)
	}
}

func TestLanguageMapManagement(t *testing.T) {
	// Test adding language to map
	metadata := make(map[string]string)
	languages := []string{"en-US", "fr-FR"}

	updatedMetadata := frame.LanguageToMap(metadata, languages)

	if updatedMetadata["lang"] != "en-US,fr-FR" {
		t.Errorf("LanguageToMap did not set the expected value: got %s, want %s",
			updatedMetadata["lang"], "en-US,fr-FR")
	}

	// Test retrieving language from map
	extractedLangs := frame.LanguageFromMap(updatedMetadata)

	if !reflect.DeepEqual(languages, extractedLangs) {
		t.Errorf("LanguageFromMap did not return the expected languages: got %v, want %v",
			extractedLangs, languages)
	}

	// Test with empty map
	emptyMetadata := make(map[string]string)
	emptyLangs := frame.LanguageFromMap(emptyMetadata)
	if emptyLangs != nil {
		t.Errorf("LanguageFromMap on empty map should return nil, got %v", emptyLangs)
	}
}

func TestLanguageHTTPMiddleware(t *testing.T) {
	translations := frame.WithTranslations("tests_runner/localization", "en", "sw")
	_, srv := frame.NewService("Test HTTP Middleware", translations)

	// Create a simple handler that checks for language in context
	handlerCalled := false
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		langs := frame.LanguageFromContext(r.Context())
		if len(langs) == 0 {
			t.Error("Expected languages in context but got none")
		}
		if langs[0] != "fr-FR" {
			t.Errorf("Expected first language to be fr-FR, got %s", langs[0])
		}
	})

	// Create middleware
	middleware := srv.LanguageHTTPMiddleware(handler)

	// Create test request with Accept-Language header
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Add("Accept-Language", "fr-FR,en-US")

	// Create response recorder
	rr := httptest.NewRecorder()

	// Serve request through middleware
	middleware.ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("Handler was not called by middleware")
	}

	// Test with lang query parameter
	handlerCalled = false
	req = httptest.NewRequest(http.MethodGet, "/test?lang=de-DE", nil)
	req.Header.Add("Accept-Language", "fr-FR,en-US")

	handler = http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		langs := frame.LanguageFromContext(r.Context())
		if len(langs) == 0 {
			t.Error("Expected languages in context but got none")
		}
		if langs[0] != "de-DE" {
			t.Errorf("Expected first language to be de-DE, got %s", langs[0])
		}
	})

	middleware = srv.LanguageHTTPMiddleware(handler)
	rr = httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("Handler was not called by middleware")
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

func TestLanguageGrpcInterceptors(t *testing.T) {
	translations := frame.WithTranslations("tests_runner/localization", "en", "sw")
	_, srv := frame.NewService("Test GRPC Interceptors", translations)

	// Test unary interceptor
	unaryInterceptor := srv.LanguageUnaryInterceptor()

	// Create metadata with Accept-Language
	md := metadata.Pairs("accept-language", "de-DE,fr-FR")
	ctx := metadata.NewIncomingContext(t.Context(), md)

	handlerCalled := false
	unaryHandler := func(ctx context.Context, _ any) (any, error) {
		handlerCalled = true
		langs := frame.LanguageFromContext(ctx)
		if len(langs) == 0 {
			t.Error("Expected languages in context but got none")
		}
		if langs[0] != "de-DE" {
			t.Errorf("Expected first language to be de-DE, got %s", langs[0])
		}
		return "response", nil
	}

	_, err := unaryInterceptor(ctx, "request", &grpc.UnaryServerInfo{}, unaryHandler)
	if err != nil {
		t.Errorf("Unary interceptor returned error: %v", err)
	}

	if !handlerCalled {
		t.Error("Unary handler was not called by interceptor")
	}

	// Test stream interceptor
	streamInterceptor := srv.LanguageStreamInterceptor()

	streamCtx := metadata.NewIncomingContext(t.Context(), md)
	mockStream := &mockServerStream{ctx: streamCtx}

	streamHandlerCalled := false
	streamHandler := func(_ any, stream grpc.ServerStream) error {
		streamHandlerCalled = true
		langs := frame.LanguageFromContext(stream.Context())
		if len(langs) == 0 {
			t.Error("Expected languages in context but got none")
		}
		if langs[0] != "de-DE" {
			t.Errorf("Expected first language to be de-DE, got %s", langs[0])
		}
		return nil
	}

	err = streamInterceptor("service", mockStream, &grpc.StreamServerInfo{}, streamHandler)
	if err != nil {
		t.Errorf("Stream interceptor returned error: %v", err)
	}

	if !streamHandlerCalled {
		t.Error("Stream handler was not called by interceptor")
	}
}

func TestLanguageFromGrpcRequest(t *testing.T) {
	// Test with valid metadata
	md := metadata.Pairs("accept-language", "ja-JP,en-US")
	ctx := metadata.NewIncomingContext(t.Context(), md)

	langs := frame.LanguageFromGrpcRequest(ctx)
	if len(langs) != 2 {
		t.Errorf("Expected 2 languages, got %d", len(langs))
	}

	if langs[0] != "ja-JP" {
		t.Errorf("Expected first language to be ja-JP, got %s", langs[0])
	}

	// Test with no metadata
	emptyCtx := t.Context()
	emptyLangs := frame.LanguageFromGrpcRequest(emptyCtx)
	if len(emptyLangs) != 0 {
		t.Errorf("Expected empty language slice, got %v", emptyLangs)
	}

	// Test with metadata but no accept-language
	mdWithoutLang := metadata.Pairs("other-header", "some-value")
	ctxWithoutLang := metadata.NewIncomingContext(t.Context(), mdWithoutLang)

	langsEmpty := frame.LanguageFromGrpcRequest(ctxWithoutLang)
	if len(langsEmpty) != 0 {
		t.Errorf("Expected empty language slice, got %v", langsEmpty)
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

func TestQueueLanguagePropagation(t *testing.T) {
	// Test publisher language propagation
	ctx := t.Context()
	languages := []string{"es-ES", "pt-BR"}

	// Add languages to context
	ctxWithLang := frame.LangugageToContext(ctx, languages)

	// Create md map
	md := make(map[string]string)

	// Extract language from context and add to md (simulating publisher.Publish)
	language := frame.LanguageFromContext(ctxWithLang)
	if len(language) > 0 {
		md = frame.LanguageToMap(md, language)
	}

	// Verify md has the language
	if md["lang"] != "es-ES,pt-BR" {
		t.Errorf("Expected language md to be 'es-ES,pt-BR', got '%s'", md["lang"])
	}

	// Test subscriber language propagation
	mockMsg := &mockMessage{
		metadata: md,
		body:     []byte("test message"),
	}

	// Extract language from md and add to context (simulating subscriber.processReceivedMessage)
	processedCtx := t.Context()
	languages = frame.LanguageFromMap(mockMsg.Metadata())
	if len(languages) > 0 {
		processedCtx = frame.LangugageToContext(processedCtx, languages)
	}

	// Verify context has the language
	extractedLangs := frame.LanguageFromContext(processedCtx)
	if !reflect.DeepEqual(extractedLangs, []string{"es-ES", "pt-BR"}) {
		t.Errorf("Expected languages [es-ES pt-BR], got %v", extractedLangs)
	}
}
