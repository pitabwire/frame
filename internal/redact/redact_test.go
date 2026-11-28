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

// redis://<token>@host and nats://<token>@host carry the credential as a
// bare userinfo, so a username without a password is masked too.
func TestBareUserinfoIsTreatedAsSecret(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, raw, token, want string
	}{
		{name: "redis token", raw: "redis://tok-redis-1@h", token: "tok-redis-1", want: "redis://xxxxx@h"},
		{name: "nats token", raw: "nats://tok-nats-2@h/x", token: "tok-nats-2", want: "nats://xxxxx@h/x"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, redact.URL(tc.raw))

			text := fmt.Sprintf("dial %s failed; user %q rejected", tc.raw, tc.token)
			require.NotContains(t, redact.Text(tc.raw, text), tc.token)

			err := redact.Error(tc.raw, fmt.Errorf("open %q: %w", tc.raw, errors.New("refused")))
			require.NotContains(t, err.Error(), tc.token)
		})
	}

	require.Equal(t, "nats://svc:xxxxx@h:4222", redact.URL("nats://svc:pw@h:4222"))
}

// An '@' after the first path/query/fragment delimiter is treated as a
// mis-split credential: legitimate URLs with '@' there are over-redacted,
// never under-redacted.
func TestURL_AtSignAfterDelimiterIsOverRedacted(t *testing.T) {
	t.Parallel()
	require.Equal(t, "https://xxxxx@b.com", redact.URL("https://api.example.com/invite?email=a@b.com"))
	require.Equal(t, "https://api.example.com/invite?email=a", redact.URL("https://api.example.com/invite?email=a"))
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

func TestBody_MasksKeyVariantsValueShapesAndTruncation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		body    string
		secrets []string
		keep    []string
	}{
		{name: "camelCase clientSecret", body: `{"clientSecret":"b-cs1"}`, secrets: []string{"b-cs1"}},
		{name: "camelCase refreshToken", body: `{"refreshToken":"b-rt2"}`, secrets: []string{"b-rt2"}},
		{name: "camelCase accessToken", body: `{"accessToken":"b-at3"}`, secrets: []string{"b-at3"}},
		{name: "camelCase newPassword", body: `{"newPassword":"b-np4"}`, secrets: []string{"b-np4"}},
		{name: "camelCase idToken", body: `{"idToken":"b-it5"}`, secrets: []string{"b-it5"}},
		{name: "camelCase apiKey", body: `{"apiKey":"b-ak6"}`, secrets: []string{"b-ak6"}},
		{name: "snake new_password", body: `{"new_password":"b-np7"}`, secrets: []string{"b-np7"}},
		{name: "snake client_secret", body: `{"client_secret":"b-cs8"}`, secrets: []string{"b-cs8"}},
		{name: "mixed case key with spaces", body: `{"Password" : "b-pw9"}`, secrets: []string{"b-pw9"}},
		{name: "nested object", body: `{"user":{"password":"b-pw10"},"name":"x"}`,
			secrets: []string{"b-pw10"}, keep: []string{`"name":"x"`}},
		{name: "numeric value", body: `{"otp":123456,"n":1}`, secrets: []string{"123456"}, keep: []string{`"n":1`}},
		{name: "array value", body: `{"password":["b-arr11","b-arr12"],"ok":true}`,
			secrets: []string{"b-arr11", "b-arr12"}, keep: []string{`"ok":true`}},
		{name: "escaped quote in value", body: `{"password":"p\"w-b13","k":"v"}`,
			secrets: []string{`p\"w-b13`, "w-b13"}, keep: []string{`"k":"v"`}},
		{name: "truncated json string", body: `{"password":"b-trunc14`, secrets: []string{"b-trunc14"}},
		{name: "truncated json array", body: `{"password":["b-trunc15`, secrets: []string{"b-trunc15"}},
		{name: "oauth form code", body: `grant_type=authorization_code&code=b-code16&code_verifier=b-cv17&client_id=c`,
			secrets: []string{"b-code16", "b-cv17"}, keep: []string{"client_id=c", "grant_type=authorization_code"}},
		{name: "form camelCase", body: "clientSecret=b-cs18&x=1", secrets: []string{"b-cs18"}, keep: []string{"x=1"}},
		{name: "truncated form value", body: "username=u&password=b-trunc19", secrets: []string{"b-trunc19"}},
		{name: "form value then newline", body: "password=b-nl20\n", secrets: []string{"b-nl20"}},
		{name: "benign keys untouched", body: `{"token_type":"bearer","expires_in":3600,"code":404}`,
			keep: []string{`"token_type":"bearer"`, `"expires_in":3600`, `"code":404`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := redact.Body(tc.body)
			for _, s := range tc.secrets {
				require.NotContains(t, got, s, "body=%q", tc.body)
			}
			for _, k := range tc.keep {
				require.Contains(t, got, k, "body=%q", tc.body)
			}
		})
	}
}
