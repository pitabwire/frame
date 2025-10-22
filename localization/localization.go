package localization

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pitabwire/util"
	"golang.org/x/text/language"
	"google.golang.org/grpc/metadata"
)

type contextKey string

func (c contextKey) String() string {
	return "frame/localization/" + string(c)
}

const ctxKeyLanguage = contextKey("languageKey")

// ToContext adds language to the current supplied context.
func ToContext(ctx context.Context, lang []string) context.Context {
	return context.WithValue(ctx, ctxKeyLanguage, lang)
}

// FromContext extracts language from the supplied context if any exist.
func FromContext(ctx context.Context) []string {
	languages, ok := ctx.Value(ctxKeyLanguage).([]string)
	if !ok {
		return nil
	}

	return languages
}

func ToMap(m map[string]string, lang []string) map[string]string {
	m["lang"] = strings.Join(lang, ",")
	return m
}

func FromMap(m map[string]string) []string {
	lang, ok := m["lang"]
	if !ok {
		return nil
	}
	return strings.Split(lang, ",")
}

type Manager interface {
	Bundle() *i18n.Bundle
	Translate(ctx context.Context, request any, messageID string) string
	TranslateWithMap(
		ctx context.Context,
		request any,
		messageID string,
		variables map[string]any,
	) string
	TranslateWithMapAndCount(
		ctx context.Context,
		request any,
		messageID string,
		variables map[string]any,
		count int,
	) string
}

type managerImpl struct {
	bundle *i18n.Bundle
}

// NewManager Option to initialize/loadOIDC different language packs.
func NewManager(translationsFolder string, languages ...string) Manager {
	if translationsFolder == "" {
		translationsFolder = "localization"
	}

	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, lang := range languages {
		bundle.MustLoadMessageFile(fmt.Sprintf("%s/messages.%v.toml", translationsFolder, lang))
	}

	return &managerImpl{bundle: bundle}
}

// Bundle Access the translation bundle instatiated in the system.
func (s *managerImpl) Bundle() *i18n.Bundle {
	return s.bundle
}

// Translate performs a quick translation based on the supplied message id.
func (s *managerImpl) Translate(ctx context.Context, request any, messageID string) string {
	return s.TranslateWithMap(ctx, request, messageID, map[string]any{})
}

// TranslateWithMap performs a translation with variables based on the supplied message id.
func (s *managerImpl) TranslateWithMap(
	ctx context.Context,
	request any,
	messageID string,
	variables map[string]any,
) string {
	return s.TranslateWithMapAndCount(ctx, request, messageID, variables, 1)
}

// TranslateWithMapAndCount performs a translation with variables based on the supplied message id and can pluralize.
func (s *managerImpl) TranslateWithMapAndCount(
	ctx context.Context,
	request any,
	messageID string,
	variables map[string]any,
	count int,
) string {
	var languageSlice []string

	switch v := request.(type) {
	case *http.Request:

		languageSlice = ExtractLanguageFromHTTPRequest(v)

	case context.Context:
		languageSlice = ExtractLanguageFromGrpcRequest(v)

	case string:
		languageSlice = []string{v}

	case []string:
		languageSlice = v

	default:
		logger := util.Log(ctx).WithField("messageID", messageID).WithField("variables", variables)
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
		logger := util.Log(ctx).WithError(err)
		logger.Error(" TranslateWithMapAndCount -- could not perform translation")
	}

	return transVersion
}

func ExtractLanguageFromHTTPRequest(req *http.Request) []string {
	lang := req.FormValue("lang")

	acceptedLang := ExtractLanguageFromHTTPHeader(req.Header)

	var lanugages []string
	if lang != "" {
		lanugages = append(lanugages, lang)
	}

	return append(lanugages, acceptedLang...)
}

func ExtractLanguageFromHTTPHeader(req http.Header) []string {

	acceptLanguageHeader := req.Get("Accept-Language")
	return strings.Split(acceptLanguageHeader, ",")
}

func ExtractLanguageFromGrpcRequest(ctx context.Context) []string {
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
