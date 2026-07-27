package config_test

import (
	"testing"

	"github.com/pitabwire/frame/v2/config"
	"github.com/stretchr/testify/require"
)

func TestParseAudienceBaseURL(t *testing.T) {
	t.Parallel()

	baseURL, err := config.ParseAudienceBaseURL("https://API.EXAMPLE.ORG/platform/")
	require.NoError(t, err)
	require.Equal(t, config.AudienceBaseURL("https://api.example.org/platform"), baseURL)

	for _, value := range []string{
		"", "http://api.example.org", "https://api.example.org:443",
		"https://user@api.example.org", "https://api.example.org/a/../b",
		"https://api.example.org?x=1", "https://api.example.org/%70latform",
	} {
		_, parseErr := config.ParseAudienceBaseURL(value)
		require.Error(t, parseErr, value)
	}
}

func TestParseResourceAudience(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    config.ResourceAudience
		wantErr bool
	}{
		{name: "canonical path", value: "https://api.stawi.org/profile", want: "https://api.stawi.org/profile"},
		{name: "canonical subdomain", value: "https://profile.stawi.org", want: "https://profile.stawi.org"},
		{name: "normalizes host path", value: " https://API.STAWI.ORG/profile ", want: "https://api.stawi.org/profile"},
		{name: "normalizes host subdomain", value: " https://PROFILE.STAWI.ORG ", want: "https://profile.stawi.org"},
		{name: "empty", value: "", wantErr: true},
		{name: "non url", value: "service_profile", wantErr: true},
		{name: "http", value: "http://profile.stawi.org", wantErr: true},
		{name: "apex only", value: "https://stawi.org", want: "https://stawi.org"},
		{name: "port", value: "https://profile.stawi.org:443", wantErr: true},
		{name: "query", value: "https://profile.stawi.org?a=b", wantErr: true},
		{name: "fragment", value: "https://profile.stawi.org#x", wantErr: true},
		{name: "userinfo", value: "https://user@profile.stawi.org", wantErr: true},
		{name: "trailing slash path", value: "https://api.stawi.org/profile/", wantErr: true},
		{name: "dot segment", value: "https://api.stawi.org/a/../profile", wantErr: true},
		{name: "duplicate slash", value: "https://api.stawi.org//profile", wantErr: true},
		{name: "encoded", value: "https://api.stawi.org/%70rofile", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := config.ParseResourceAudience(test.value)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestParseResourceAudiencesRejectsDuplicatesAndSorts(t *testing.T) {
	t.Parallel()

	got, err := config.ParseResourceAudiences([]string{
		"https://tenancy.stawi.org",
		"https://profile.stawi.org",
	})
	require.NoError(t, err)
	require.Equal(t, []config.ResourceAudience{
		"https://profile.stawi.org",
		"https://tenancy.stawi.org",
	}, got)

	_, err = config.ParseResourceAudiences([]string{
		"https://profile.stawi.org",
		"https://PROFILE.STAWI.ORG",
	})
	require.Error(t, err)
}
