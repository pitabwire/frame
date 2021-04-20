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

func Translations(languages ... string) Option {

	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, lang := range languages {
		bundle.MustLoadMessageFile(fmt.Sprintf("localization/messages.%v.toml", lang))
	}

	return func(c *Service) {

		c.bundle = bundle
	}
}

