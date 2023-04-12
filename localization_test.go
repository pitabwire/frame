package frame_test

import (
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pitabwire/frame"
	"testing"
)

func TestTranslations(t *testing.T) {

	translations := frame.Translations("tests_runner/localization", "en", "sw")
	_, srv := frame.NewService("Test Localization Srv", translations)

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

	if englishVersion != "Air has nothing" {
		t.Errorf("Localizations didn't quite work like they should, found : %s expected : %s", englishVersion, "Air has nothing")
		return
	}

	swLocalizer := i18n.NewLocalizer(bundle, "sw")
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

	if swVersion != "Air haina chochote" {
		t.Errorf("Localizations didn't quite work like they should, found : %s expected : %s", swVersion, "Air haina chochote")
		return
	}

}

func TestTranslationsHelpers(t *testing.T) {

	translations := frame.Translations("tests_runner/localization", "en", "sw")
	_, srv := frame.NewService("Test Localization Srv", translations)

	englishVersion := srv.Translate("en", "Example")
	if englishVersion != "<no value> has nothing" {
		t.Errorf("Localizations didn't quite work like they should, found : %s expected : %s", englishVersion, "<no value> has nothing")
		return
	}

	englishVersion = srv.TranslateWithMap("en", "Example", map[string]interface{}{"Name": "MapMan"})
	if englishVersion != "MapMan has nothing" {
		t.Errorf("Localizations didn't quite work like they should, found : %s expected : %s", englishVersion, "MapMan has nothing")
		return
	}

	englishVersion = srv.TranslateWithMapAndCount("en", "Example", map[string]interface{}{"Name": "CountMen"}, 2)
	if englishVersion != "CountMen have nothing" {
		t.Errorf("Localizations didn't quite work like they should, found : %s expected : %s", englishVersion, "CountMen have nothing")
		return
	}

}
