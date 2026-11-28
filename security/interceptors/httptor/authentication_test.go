package httptor_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pitabwire/frame/v3/security"
	"github.com/pitabwire/frame/v3/security/interceptors/httptor"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
)

type rejectingAuthenticator struct{}

func (rejectingAuthenticator) Authenticate(
	_ context.Context, _ string, _ ...security.AuthOption,
) (context.Context, error) {
	return nil, errors.New("token is expired")
}

func captureLogs(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	logger := util.NewLogger(t.Context(),
		util.WithLogOutput(buf),
		util.WithLogLevel(slog.LevelDebug),
		util.WithLogNoColor(true),
		util.WithLogHandlerExclusive(),
	)
	return util.ContextWithLogger(t.Context(), logger), buf
}

func TestAuthenticationMiddleware_NeverLogsCredentials(t *testing.T) {
	const secretToken = "eyJhbGciOiJSUzI1NiJ9.c2VjcmV0LXBheWxvYWQ.c2lnbmF0dXJlLXNlY3JldA"

	cases := []struct {
		name       string
		headers    map[string]string
		wantStatus int
		secrets    []string
		wantFP     bool
	}{
		{
			name:       "rejected bearer token",
			headers:    map[string]string{"Authorization": "Bearer " + secretToken},
			wantStatus: http.StatusForbidden,
			secrets:    []string{secretToken},
			wantFP:     true,
		},
		{
			name: "non-bearer scheme with cookies",
			headers: map[string]string{
				"Authorization": "Basic dXNlcjpiYXNpYy1zZWNyZXQ=",
				"Cookie":        "session=cookie-secret",
			},
			wantStatus: http.StatusUnauthorized,
			secrets:    []string{"dXNlcjpiYXNpYy1zZWNyZXQ=", "cookie-secret"},
		},
		{
			name:       "malformed bearer header",
			headers:    map[string]string{"Authorization": "Bearer " + secretToken + " trailing-secret"},
			wantStatus: http.StatusBadRequest,
			secrets:    []string{secretToken, "trailing-secret"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, logs := captureLogs(t)

			handler := httptor.AuthenticationMiddleware(
				http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					t.Fatal("next handler must not run for a rejected request")
				}),
				rejectingAuthenticator{},
			)

			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/resource", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			require.Equal(t, tc.wantStatus, rec.Code)
			require.NotEmpty(t, logs.String(), "the refusal must still be logged")
			for _, secret := range tc.secrets {
				require.NotContains(t, logs.String(), secret, "credentials must never reach the logs")
			}
			if tc.wantFP {
				require.Contains(t, logs.String(), security.TokenFingerprint(secretToken),
					"a token fingerprint is logged so refusals can be correlated")
			}
		})
	}
}
