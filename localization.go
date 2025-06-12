package frame

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"google.golang.org/grpc/metadata"
)

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
		languageSlice = extractLanguageFromGrpcRequest(v)

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

	return append([]string{lang}, acceptedLang...)
}

func extractLanguageFromGrpcRequest(ctx context.Context) []string {
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
