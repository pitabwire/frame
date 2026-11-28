package redact_test

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"testing"

	"github.com/pitabwire/frame/v3/internal/redact"
	"github.com/stretchr/testify/require"
)

func TestURL_MasksPasswordsWithReservedCharacters(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		raw    string
		secret string
	}{
		{name: "slash in password", raw: "postgres://user:pa/ss@h/db", secret: "pa/ss"},
		{name: "slash with numeric prefix parses as host:port", raw: "postgres://user:12/ss@h/db", secret: "12/ss"},
		{name: "hash in password", raw: "postgres://user:pa#ss@h/db", secret: "pa#ss"},
		{name: "double quote in password", raw: `postgres://user:pa"ss@h/db`, secret: `pa"ss`},
		{name: "question mark with numeric prefix", raw: "nats://user:12?x@h:4222", secret: "12?x"},
		{name: "at sign in password", raw: "nats://user:p@ss@h:4222/s", secret: "p@ss"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := redact.URL(tc.raw)
			require.NotContains(t, got, tc.secret)
			require.NotContains(t, got, strconv.Quote(tc.secret)[1:len(strconv.Quote(tc.secret))-1])
		})
	}
}

func TestError_MasksQuotedAndReservedPasswordForms(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		raw    string
		secret string
	}{
		{name: "slash", raw: "postgres://user:pa/ss@h/db", secret: "pa/ss"},
		{name: "hash", raw: "postgres://user:pa#ss@h/db", secret: "pa#ss"},
		{name: "double quote", raw: `postgres://user:pa"ss@h/db`, secret: `pa"ss`},
		{name: "control character", raw: "nats://user:pw-ctl@h:4222/" + string(rune(0x7f)), secret: "pw-ctl"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, parseErr := url.Parse(tc.raw)
			if parseErr == nil {
				parseErr = fmt.Errorf("dial %q: refused", tc.raw)
			}
			wrapped := fmt.Errorf("open: %w", parseErr)

			got := redact.Error(tc.raw, wrapped)

			quoted := strconv.Quote(tc.secret)
			require.NotContains(t, got.Error(), tc.secret)
			require.NotContains(t, got.Error(), quoted[1:len(quoted)-1], "escaped (%q) form must be masked")
			require.ErrorIs(t, got, parseErr)
		})
	}
}

func TestURL_UserOnlyUserinfoIsNotASecret(t *testing.T) {
	t.Parallel()
	require.Equal(t, "https://alice@example.com/x", redact.URL("https://alice@example.com/x"))
	require.Equal(t, "nats://svc:xxxxx@h:4222", redact.URL("nats://svc:pw@h:4222"))

	plain := errors.New(`dial "https://alice@example.com/x": refused`)
	require.Same(t, plain, redact.Error("https://alice@example.com/x", plain),
		"a username alone is not a credential; the error must be left intact")
}

func TestURL_LegitAtSignInQueryIsKept(t *testing.T) {
	t.Parallel()
	raw := "https://api.example.com/invite?email=a@b.com"
	require.Equal(t, raw, redact.URL(raw))
}

func TestHeaderValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, header, value, want string
	}{
		{"authorization", "Authorization", "Bearer x", redact.Placeholder},
		{"lowercase cookie", "cookie", "a=b", redact.Placeholder},
		{"set-cookie", "Set-Cookie", "a=b", redact.Placeholder},
		{"api key", "X-Api-Key", "k", redact.Placeholder},
		{"benign", "Content-Type", "application/json", "application/json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, redact.HeaderValue(tc.header, tc.value))
		})
	}
}

func TestBody_MasksCredentialFields(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		body    string
		secrets []string
		keep    []string
	}{
		{
			name:    "oauth token form request",
			body:    "grant_type=client_credentials&client_id=svc&client_secret=cs-123&client_assertion=ca-456",
			secrets: []string{"cs-123", "ca-456"},
			keep:    []string{"grant_type=client_credentials", "client_id=svc"},
		},
		{
			name:    "oauth token json response",
			body:    `{"access_token":"at-789","refresh_token": "rt-000","id_token":"it-111","token_type":"bearer","expires_in":3600}`,
			secrets: []string{"at-789", "rt-000", "it-111"},
			keep:    []string{`"token_type":"bearer"`, `"expires_in":3600`},
		},
		{
			name:    "password login json",
			body:    `{"username":"bob","password":"pw \"quoted\" x"}`,
			secrets: []string{`pw \"quoted\" x`, "quoted"},
			keep:    []string{`"username":"bob"`},
		},
		{
			name: "no credentials",
			body: `{"name":"widget"}`,
			keep: []string{`{"name":"widget"}`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := redact.Body(tc.body)
			for _, s := range tc.secrets {
				require.NotContains(t, got, s)
			}
			for _, k := range tc.keep {
				require.Contains(t, got, k)
			}
		})
	}
}
