package config

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
)

const httpsScheme = "https"

type ResourceAudience string

type ClientAssertionAudience string

type AudienceBaseURL string

func ParseAudienceBaseURL(value string) (AudienceBaseURL, error) {
	value = strings.TrimSuffix(strings.TrimSpace(value), "/")
	if value == "" {
		return "", errors.New("audience base URL is required")
	}
	if strings.Contains(value, "%") {
		return "", errors.New("audience base URL must not be percent encoded")
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse audience base URL: %w", err)
	}
	if parsed.Scheme != httpsScheme || parsed.Host == "" {
		return "", errors.New("audience base URL must be an absolute HTTPS URL")
	}
	if parsed.User != nil || parsed.Port() != "" || parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" {
		return "", errors.New("audience base URL must not contain user information, a port, query, or fragment")
	}
	if parsed.Path != "" && path.Clean(parsed.Path) != parsed.Path {
		return "", errors.New("audience base URL path must be canonical")
	}

	parsed.Host = strings.ToLower(parsed.Hostname())
	parsed.RawPath = ""
	return AudienceBaseURL(strings.TrimSuffix(parsed.String(), "/")), nil
}

func ParseResourceAudience(value string) (ResourceAudience, error) {
	normalized, err := normalizeAudienceURL(value)
	if err != nil {
		return "", fmt.Errorf("resource audience: %w", err)
	}
	return ResourceAudience(normalized), nil
}

func ParseClientAssertionAudience(value string) (ClientAssertionAudience, error) {
	normalized, err := normalizeAudienceURL(value)
	if err != nil {
		return "", fmt.Errorf("client assertion audience: %w", err)
	}
	return ClientAssertionAudience(normalized), nil
}

func ParseResourceAudiences(values []string) ([]ResourceAudience, error) {
	if len(values) == 0 {
		return nil, nil
	}

	seen := make(map[ResourceAudience]struct{}, len(values))
	audiences := make([]ResourceAudience, 0, len(values))
	for index, value := range values {
		audience, err := ParseResourceAudience(value)
		if err != nil {
			return nil, fmt.Errorf("audience %d: %w", index, err)
		}
		if _, exists := seen[audience]; exists {
			return nil, fmt.Errorf("audience %d: duplicate audience %q", index, audience)
		}
		seen[audience] = struct{}{}
		audiences = append(audiences, audience)
	}

	sort.Slice(audiences, func(i, j int) bool { return audiences[i] < audiences[j] })
	return audiences, nil
}

func ResourceAudienceStrings(audiences []ResourceAudience) []string {
	values := make([]string, len(audiences))
	for index, audience := range audiences {
		values[index] = string(audience)
	}
	return values
}

func normalizeAudienceURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("URL is required")
	}
	if strings.Contains(value, "%") {
		return "", errors.New("percent-encoded URLs are not allowed")
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse URL: %w", err)
	}
	if parsed.Scheme != httpsScheme {
		return "", errors.New("scheme must be https")
	}
	if parsed.Host == "" {
		return "", errors.New("host is required")
	}
	if parsed.User != nil {
		return "", errors.New("user information is not allowed")
	}
	if parsed.Port() != "" {
		return "", errors.New("port is not allowed")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return "", errors.New("query is not allowed")
	}
	if parsed.Fragment != "" {
		return "", errors.New("fragment is not allowed")
	}
	if parsed.Path == "" || parsed.Path == "/" {
		return "", errors.New("non-root path is required")
	}
	if strings.HasSuffix(parsed.Path, "/") {
		return "", errors.New("trailing slash is not allowed")
	}
	if cleaned := path.Clean(parsed.Path); cleaned != parsed.Path {
		return "", errors.New("path must not contain dot or duplicate-slash segments")
	}

	parsed.Scheme = httpsScheme
	parsed.Host = strings.ToLower(parsed.Hostname())
	parsed.RawPath = ""
	return parsed.String(), nil
}
