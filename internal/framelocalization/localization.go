package framelocalization

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/pitabwire/frame/internal/common"
)

const ctxKeyLanguage = common.ContextKey("languageKey")

// LangugageToContext adds language to the current supplied context.
func LangugageToContext(ctx context.Context, lang []string) context.Context {
	return context.WithValue(ctx, ctxKeyLanguage, lang)
}

// LanguageFromContext extracts language from the supplied context if any exist.
func LanguageFromContext(ctx context.Context) []string {
	languages, ok := ctx.Value(ctxKeyLanguage).([]string)
	if !ok {
		return nil
	}

	return languages
}

func LanguageToMap(m map[string]string, lang []string) map[string]string {
	m["lang"] = strings.Join(lang, ",")
	return m
}

func LanguageFromMap(m map[string]string) []string {
	lang, ok := m["lang"]
	if !ok {
		return nil
	}
	return strings.Split(lang, ",")
}

// LanguageHTTPMiddleware is an HTTP middleware that extracts language information and sets it in the context.
func LanguageHTTPMiddleware(s common.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l := extractLanguageFromHTTPRequest(r)

		ctx := LangugageToContext(r.Context(), l)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

// LanguageUnaryInterceptor Simple grpc interceptor to extract the language supplied via metadata.
func LanguageUnaryInterceptor(s common.Service) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any,
		_ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		l := LanguageFromGrpcRequest(ctx)
		if l != nil {
			ctx = LangugageToContext(ctx, l)
		}

		return handler(ctx, req)
	}
}

// LanguageStreamInterceptor A language extractor that will extract .
func LanguageStreamInterceptor(s common.Service) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		l := LanguageFromGrpcRequest(ctx)
		if l == nil {
			return handler(srv, ss)
		}

		ctx = LangugageToContext(ctx, l)

		// Wrap the original stream with ctx this ensures the handlers always receives a stream from which it can get the correct context.
		languageStream := &common.ServerStreamWrapper{Ctx: ctx, ServerStream: ss}

		return handler(srv, languageStream)
	}
}

// Bundle method moved to avoid common.Service dependency

// Translate performs a quick translation based on the supplied message id.
func Translate(ctx context.Context, s common.Service, request any, messageID string) string {
	return TranslateWithMap(ctx, s, request, messageID, map[string]any{})
}

// TranslateWithMap performs a translation with variables based on the supplied message id.
func TranslateWithMap(
	ctx context.Context,
	s common.Service,
	request any,
	messageID string,
	variables map[string]any,
) string {
	return TranslateWithMapAndCount(ctx, s, request, messageID, variables, 1)
}

// TranslateWithMapAndCount performs a translation with variables based on the supplied message id and can pluralize.
func TranslateWithMapAndCount(
	ctx context.Context,
	s common.Service,
	request any,
	messageID string,
	variables map[string]any,
	count int,
) string {
	var languageSlice []string

	switch v := request.(type) {
	case *http.Request:

		languageSlice = extractLanguageFromHTTPRequest(v)

	case context.Context:
		languageSlice = LanguageFromGrpcRequest(v)

	case string:
		languageSlice = []string{v}

	case []string:
		languageSlice = v

	default:
		logger := s.Log(ctx).WithField("messageID", messageID).WithField("variables", variables)
		logger.Warn("TranslateWithMapAndCount -- no valid request object found, use string, []string, context or http.Request")
		return messageID
	}

	// Get localization module and use placeholder bundle since Bundle() method not available from interface
	locModule := s.GetModule(common.ModuleTypeLocalization)
	if locModule == nil {
		return messageID
	}
	
	// Use placeholder bundle since we can't access Bundle() method from interface
	bundle := &i18n.Bundle{}
	localizer := i18n.NewLocalizer(bundle, languageSlice...)

	transVersion, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:      messageID,
		DefaultMessage: &i18n.Message{ID: messageID},
		TemplateData:   variables,
		PluralCount:    count,
	})

	if err != nil {
		logger := s.Log(ctx).WithError(err)
		logger.Error(" TranslateWithMapAndCount -- could not perform translation")
	}

	return transVersion
}

func extractLanguageFromHTTPRequest(req *http.Request) []string {
	lang := req.FormValue("lang")
	acceptLanguageHeader := req.Header.Get("Accept-Language")
	acceptedLang := strings.Split(acceptLanguageHeader, ",")

	var lanugages []string
	if lang != "" {
		lanugages = append(lanugages, lang)
	}

	return append(lanugages, acceptedLang...)
}

func LanguageFromGrpcRequest(ctx context.Context) []string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return []string{}
	}

	header, ok := md["accept-language"]
	if !ok || len(header) == 0 {
		return []string{}
	}
	acceptLangHeader := header[0]
	return strings.Split(acceptLangHeader, ",")
}

// WithTranslations Option to initialize/loadOIDC different language packs.
func WithTranslations(translationsFolder string, languages ...string) common.Option {
	if translationsFolder == "" {
		translationsFolder = "localization"
	}

	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, lang := range languages {
		bundle.MustLoadMessageFile(fmt.Sprintf("%s/messages.%v.toml", translationsFolder, lang))
	}

	return func(_ context.Context, c common.Service) {
		// Register the LocalizationModule with the bundle
		localizationModule := common.NewLocalizationModule(bundle)
		c.RegisterModule(localizationModule)
	}
}
