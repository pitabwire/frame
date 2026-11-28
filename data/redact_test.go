package data_test

import (
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/pitabwire/frame/v3/data"
	"github.com/stretchr/testify/require"
)

func TestDSNRedacted_RemovesCredentials(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		dsn     string
		want    string
		secrets []string
	}{
		{
			name:    "nats url with user and password",
			dsn:     "nats://svc-user:s3cr3t-pw@nats.queue.svc:4222/profile?jetstream=true",
			want:    "nats://svc-user:xxxxx@nats.queue.svc:4222/profile?jetstream=true",
			secrets: []string{"s3cr3t-pw"},
		},
		{
			name:    "postgres url with password",
			dsn:     "postgres://app:pg-pass@db:5432/app?sslmode=require",
			want:    "postgres://app:xxxxx@db:5432/app?sslmode=require",
			secrets: []string{"pg-pass"},
		},
		{
			name:    "token-only userinfo is redacted",
			dsn:     "redis://bearer-token-value@cache:6379/0",
			want:    "redis://xxxxx@cache:6379/0",
			secrets: []string{"bearer-token-value"},
		},
		{
			name:    "sensitive query params are redacted",
			dsn:     "nats://nats:4222/q?password=qp-secret&token=qt-secret&subject=orders",
			secrets: []string{"qp-secret", "qt-secret"},
		},
		{
			name:    "libpq keyword form password",
			dsn:     "host=db port=5432 user=app password=kv-secret dbname=app",
			want:    "host=db port=5432 user=app password=xxxxx dbname=app",
			secrets: []string{"kv-secret"},
		},
		{
			name:    "unparseable url still has userinfo redacted",
			dsn:     "nats://svc:bad-parse-secret@host:4222/\x7f",
			secrets: []string{"bad-parse-secret"},
		},
		{
			name: "url without credentials is unchanged",
			dsn:  "mem://events",
			want: "mem://events",
		},
		{
			name: "empty stays empty",
			dsn:  "",
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := data.DSN(tc.dsn).Redacted()
			if tc.want != "" || tc.dsn == "" {
				require.Equal(t, tc.want, got)
			}
			for _, secret := range tc.secrets {
				require.NotContains(t, got, secret, "redacted DSN must not contain credentials")
			}
		})
	}
}

func TestRedactURL_MatchesDSNRedacted(t *testing.T) {
	t.Parallel()
	raw := "nats://u:p4ss@h:4222"
	require.Equal(t, data.DSN(raw).Redacted(), data.RedactURL(raw))
	require.NotContains(t, data.RedactURL(raw), "p4ss")
}

func TestDSNScrubError_RemovesCredentialsFromErrorText(t *testing.T) {
	t.Parallel()

	controlChar := string(rune(0x7f))
	raw := "nats://svc:scrub-secret@host:4222/" + controlChar
	_, parseErr := url.Parse(raw)
	require.Error(t, parseErr)
	require.Contains(t, parseErr.Error(), "scrub-secret", "precondition: url.Parse leaks the password")

	escaped := "nats://svc:p%40ss-esc@host:4222"
	sentinel := errors.New("dial failed")

	cases := []struct {
		name    string
		dsn     data.DSN
		err     error
		secrets []string
	}{
		{name: "url.Parse error", dsn: data.DSN(raw), err: fmt.Errorf("queue: invalid url: %w", parseErr),
			secrets: []string{"scrub-secret"}},
		{
			name: "driver error echoing decoded password",
			dsn:  data.DSN(escaped),
			err: fmt.Errorf(
				"connect with password p@ss-esc: %w",
				sentinel,
			),
			secrets: []string{"p@ss-esc", "p%40ss-esc"},
		},
		{name: "driver error echoing url", dsn: data.DSN(escaped),
			err: fmt.Errorf("open %s: %w", escaped, sentinel), secrets: []string{"p@ss-esc", "p%40ss-esc"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			scrubbed := tc.dsn.ScrubError(tc.err)
			require.Error(t, scrubbed)
			for _, secret := range tc.secrets {
				require.NotContains(t, scrubbed.Error(), secret)
			}
			require.ErrorIs(
				t,
				scrubbed,
				errors.Unwrap(tc.err),
				"scrubbed error must keep the wrapped chain for errors.Is",
			)
		})
	}

	require.NoError(t, data.DSN(raw).ScrubError(nil))
	plain := errors.New("no secrets here")
	require.Same(t, plain, data.DSN("mem://x").ScrubError(plain), "errors without secrets are returned unchanged")
}

func TestDSNHelpers_ParseErrorsNeverLeakCredentials(t *testing.T) {
	t.Parallel()
	const password = "helper-pw-42"
	dsn := data.DSN("postgres://app:" + password + "@db:bad port/app")

	_, err := dsn.ToURI()
	require.Error(t, err)
	require.NotContains(t, err.Error(), password)

	_, err = dsn.WithPath("/other")
	require.Error(t, err)
	require.NotContains(t, err.Error(), password)

	_, err = dsn.ChangePort("6543")
	require.Error(t, err)
	require.NotContains(t, err.Error(), password)
}
