package config_test

import (
	"testing"

	"github.com/pitabwire/frame/config"
	"github.com/stretchr/testify/require"
)

func TestParseResourceAudience(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    config.ResourceAudience
		wantErr bool
	}{
		{name: "canonical", value: "https://api.stawi.org/profile", want: "https://api.stawi.org/profile"},
		{name: "normalizes host", value: " https://API.STAWI.ORG/profile ", want: "https://api.stawi.org/profile"},
		{name: "empty", value: "", wantErr: true},
		{name: "non url", value: "service_profile", wantErr: true},
		{name: "http", value: "http://api.stawi.org/profile", wantErr: true},
		{name: "root", value: "https://api.stawi.org/", wantErr: true},
		{name: "port", value: "https://api.stawi.org:443/profile", wantErr: true},
		{name: "query", value: "https://api.stawi.org/profile?a=b", wantErr: true},
		{name: "fragment", value: "https://api.stawi.org/profile#x", wantErr: true},
		{name: "userinfo", value: "https://user@api.stawi.org/profile", wantErr: true},
		{name: "trailing slash", value: "https://api.stawi.org/profile/", wantErr: true},
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
		"https://api.stawi.org/tenancy",
		"https://api.stawi.org/profile",
	})
	require.NoError(t, err)
	require.Equal(t, []config.ResourceAudience{
		"https://api.stawi.org/profile",
		"https://api.stawi.org/tenancy",
	}, got)

	_, err = config.ParseResourceAudiences([]string{
		"https://api.stawi.org/profile",
		"https://API.STAWI.ORG/profile",
	})
	require.Error(t, err)
}
