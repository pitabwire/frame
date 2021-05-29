package frame

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

func (s *Service) Bundle() *i18n.Bundle {
	return s.bundle
}

func (s *Service) Translate(lang string, messageId string) (string, error) {
	return s.TranslateWithMap(lang, messageId, map[string]interface{}{})
}

func (s *Service) TranslateWithMap(lang string, messageId string, variables map[string]interface{}) (string, error) {
	return s.TranslateWithMapAndCount(lang, messageId, variables, 1)
}

func (s *Service) TranslateWithMapAndCount(lang string, messageId string, variables map[string]interface{}, count int) (string, error) {

	if val, ok := s.localizerMap[lang]; !ok {

		val = i18n.NewLocalizer(s.Bundle(), lang)
		s.localizerMap[lang] = val
	}

	localizer := s.localizerMap[lang]

	transVersion, err := localizer.Localize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{ID: messageId},
		TemplateData:   variables,
		PluralCount:    count,
	})
	return transVersion, err

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
