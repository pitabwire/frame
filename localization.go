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

func Translations(translationsFolder string, languages ... string) Option {

	if translationsFolder == ""{
		translationsFolder = "localization"
	}

	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, lang := range languages {
		bundle.MustLoadMessageFile(fmt.Sprintf("%s/messages.%v.toml",translationsFolder, lang))
	}

	return func(c *Service) {

		c.bundle = bundle
	}
}

