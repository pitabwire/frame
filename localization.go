package frame

import (
	"context"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"google.golang.org/grpc/metadata"
	"log"
	"net/http"
	"strings"
)

func (s *Service) Bundle() *i18n.Bundle {
	return s.bundle
}

func (s *Service) Translate(request interface{}, messageId string) string {
	return s.TranslateWithMap(request, messageId, map[string]interface{}{})
}

func (s *Service) TranslateWithMap(request interface{}, messageId string, variables map[string]interface{}) string {
	return s.TranslateWithMapAndCount(request, messageId, variables, 1)
}

func (s *Service) TranslateWithMapAndCount(request interface{}, messageId string, variables map[string]interface{}, count int) string {

	var languageSlice []string

	switch v := request.(type) {
	case *http.Request:

		languageSlice = extractLanguageFromHttpRequest(v)
		break
	case context.Context:
		languageSlice = extractLanguageFromGrpcRequest(v)
		break
	case string:
		languageSlice = []string{v}
		break
	case []string:
		languageSlice = v
		break
	default:
		log.Printf("TranslateWithMapAndCount -- no valid request object found, use string, []string, context or http.Request")
		return messageId
	}

	localizer := i18n.NewLocalizer(s.Bundle(), languageSlice...)

	transVersion, err := localizer.Localize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{ID: messageId},
		TemplateData:   variables,
		PluralCount:    count,
	})

	if err != nil {
		log.Printf(" TranslateWithMapAndCount -- translation problem %+v", err)
	}

	return transVersion

}

func extractLanguageFromHttpRequest(req *http.Request) []string {

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

func Translations(translationsFolder string, languages ...string) Option {

	if translationsFolder == "" {
		translationsFolder = "localization"
	}

	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, lang := range languages {
		bundle.MustLoadMessageFile(fmt.Sprintf("%s/messages.%v.toml", translationsFolder, lang))
	}

	return func(c *Service) {

		c.bundle = bundle
	}
}
