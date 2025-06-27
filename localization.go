package frame

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
)

const ctxKeyLanguage = contextKey("languageKey")

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
func (s *Service) LanguageHTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l := extractLanguageFromHTTPRequest(r)

		ctx := LangugageToContext(r.Context(), l)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

// LanguageUnaryInterceptor Simple grpc interceptor to extract the language supplied via metadata.
func (s *Service) LanguageUnaryInterceptor() grpc.UnaryServerInterceptor {
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
func (s *Service) LanguageStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		l := LanguageFromGrpcRequest(ctx)
		if l == nil {
			return handler(srv, ss)
		}

		ctx = LangugageToContext(ctx, l)

		// Wrap the original stream with ctx this ensures the handlers always receives a stream from which it can get the correct context.
		languageStream := &serverStreamWrapper{ctx, ss}

		return handler(srv, languageStream)
	}
}

// Bundle Access the translation bundle instatiated in the system.
func (s *Service) Bundle() *i18n.Bundle {
	return s.bundle
}

// Translate performs a quick translation based on the supplied message id.
func (s *Service) Translate(ctx context.Context, request any, messageID string) string {
	return s.TranslateWithMap(ctx, request, messageID, map[string]any{})
}

// TranslateWithMap performs a translation with variables based on the supplied message id.
func (s *Service) TranslateWithMap(
	ctx context.Context,
	request any,
	messageID string,
	variables map[string]any,
) string {
	return s.TranslateWithMapAndCount(ctx, request, messageID, variables, 1)
}

// TranslateWithMapAndCount performs a translation with variables based on the supplied message id and can pluralize.
func (s *Service) TranslateWithMapAndCount(
	ctx context.Context,
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

	localizer := i18n.NewLocalizer(s.Bundle(), languageSlice...)

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
func WithTranslations(translationsFolder string, languages ...string) Option {
	if translationsFolder == "" {
		translationsFolder = "localization"
	}

	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, lang := range languages {
		bundle.MustLoadMessageFile(fmt.Sprintf("%s/messages.%v.toml", translationsFolder, lang))
	}

	return func(_ context.Context, c *Service) {
		c.bundle = bundle
	}
}
