package auth

import (
	"fmt"
	"strings"

	"github.com/pitabwire/frame/config"
)

func resolveRequestedAudiences(cfg config.ConfigurationOAUTH2) ([]string, error) {
	audiences, err := config.ParseResourceAudiences(cfg.GetOauth2RequestedAudiences())
	if err != nil {
		return nil, fmt.Errorf("oauth2 requested audiences: %w", err)
	}
	return config.ResourceAudienceStrings(audiences), nil
}

func resolveClientAssertionAudience(cfg config.ConfigurationOAUTH2, tokenURL string) (string, error) {
	value := strings.TrimSpace(cfg.GetOauth2ClientAssertionAudience())
	if value == "" {
		value = tokenURL
	}
	audience, err := config.ParseClientAssertionAudience(value)
	if err != nil {
		return "", err
	}
	return string(audience), nil
}
