package frame

import (
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"testing"
)

func TestTranslations(t *testing.T) {

	translations := Translations( "tests/localization", "en", "sw")
	srv := NewService("Test Localization Srv", translations)

	bundle := srv.Bundle()

	enLocalizer := i18n.NewLocalizer(bundle, "en", "sw")
	englishVersion, err := enLocalizer.Localize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID: "Example",
		},
		TemplateData: map[string]interface{}{
			"Name": "Air",
		},
		PluralCount: 1,
	})
	if err != nil {
		t.Errorf(" There was an error parsing the translations %+v", err)
		return
	}

	if englishVersion != "Air has nothing"{
		t.Errorf("Localizations didn't quite work like they should, found : %s expected : %s", englishVersion, "Air has nothing")
		return
	}


	swLocalizer := i18n.NewLocalizer(bundle,  "sw")
	swVersion, err := swLocalizer.Localize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID: "Example",
		},
		TemplateData: map[string]interface{}{
			"Name": "Air",
		},
		PluralCount: 1,
	})
	if err != nil {
		t.Errorf(" There was an error parsing the translations %+v", err)
		return
	}

	if swVersion != "Air haina chochote"{
		t.Errorf("Localizations didn't quite work like they should, found : %s expected : %s", swVersion, "Air haina chochote")
		return
	}



}
