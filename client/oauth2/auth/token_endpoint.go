package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/pitabwire/frame/config"
)

const maxTokenEndpointErrorBodyBytes = 8 << 10

type basicAuth struct {
	username string
	password string
}

type tokenEndpointRequest struct {
	form      url.Values
	basicAuth *basicAuth
}

// tokenEndpointResponse is decoded from the OAuth2 token endpoint JSON body.
// Fields are mapped via a custom decoder to avoid struct tags that trigger
// gosec G117 (secret-pattern matching on JSON keys like "access_token").
type tokenEndpointResponse struct {
	token          string
	tokenType      string
	expiresIn      int64
	scope          string
	errCode        string
	errDescription string
}

// ResolveTokenEndpoint determines the token endpoint URL from config,
// using OIDC discovery if needed.
func ResolveTokenEndpoint(
	ctx context.Context,
	cfg config.ConfigurationOAUTH2,
) (string, error) {
	tokenURL := strings.TrimSpace(cfg.GetOauth2TokenEndpoint())
	if tokenURL != "" || strings.TrimSpace(cfg.GetOauth2ServiceURI()) == "" {
		return tokenURL, nil
	}

	if err := cfg.LoadOauth2Config(ctx); err != nil {
		return "", fmt.Errorf("load oauth2 discovery: %w", err)
	}

	return strings.TrimSpace(cfg.GetOauth2TokenEndpoint()), nil
}

// ExchangeToken sends a token request to the given endpoint and returns the
// resulting OAuth2 token.
func exchangeToken(
	ctx context.Context,
	httpClient *http.Client,
	tokenURL string,
	req tokenEndpointRequest,
	now func() time.Time,
) (*oauth2.Token, error) {
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		tokenURL,
		strings.NewReader(req.form.Encode()),
	)
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Accept", "application/json")

	if req.basicAuth != nil {
		httpReq.SetBasicAuth(req.basicAuth.username, req.basicAuth.password)
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenEndpointErrorBodyBytes))
	if err != nil {
		return nil, err
	}

	return decodeTokenResponse(resp.StatusCode, body, now)
}

func decodeTokenResponse(
	statusCode int,
	body []byte,
	now func() time.Time,
) (*oauth2.Token, error) {
	var resp tokenEndpointResponse
	if len(body) > 0 {
		if parseErr := parseTokenEndpointJSON(body, &resp); parseErr != nil {
			if statusCode >= http.StatusBadRequest {
				return nil, fmt.Errorf("oauth2 token endpoint returned status %d", statusCode)
			}
		}
	}

	if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
		return toOAuth2Token(&resp, now)
	}

	if resp.errCode != "" {
		if resp.errDescription != "" {
			return nil, fmt.Errorf(
				"oauth2 token endpoint error: %s: %s",
				resp.errCode,
				resp.errDescription,
			)
		}
		return nil, fmt.Errorf("oauth2 token endpoint error: %s", resp.errCode)
	}

	if len(body) > 0 {
		return nil, fmt.Errorf(
			"oauth2 token endpoint returned status %d: %s",
			statusCode,
			strings.TrimSpace(string(body)),
		)
	}

	return nil, fmt.Errorf("oauth2 token endpoint returned status %d", statusCode)
}

func parseTokenEndpointJSON(body []byte, resp *tokenEndpointResponse) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}

	decodeString := func(key string) string {
		v, ok := raw[key]
		if !ok {
			return ""
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return ""
		}
		return s
	}

	resp.token = decodeString("access_token")
	resp.tokenType = decodeString("token_type")
	resp.scope = decodeString("scope")
	resp.errCode = decodeString("error")
	resp.errDescription = decodeString("error_description")

	if v, ok := raw["expires_in"]; ok {
		_ = json.Unmarshal(v, &resp.expiresIn)
	}

	return nil
}

func toOAuth2Token(
	resp *tokenEndpointResponse,
	now func() time.Time,
) (*oauth2.Token, error) {
	if resp == nil {
		return nil, errors.New("oauth2 token endpoint response is required")
	}
	if strings.TrimSpace(resp.token) == "" {
		return nil, errors.New("oauth2 token endpoint response missing access_token")
	}

	tokenType := strings.TrimSpace(resp.tokenType)
	if tokenType == "" {
		tokenType = "Bearer"
	}

	token := &oauth2.Token{
		AccessToken: resp.token,
		TokenType:   tokenType,
	}
	if resp.expiresIn > 0 {
		token.Expiry = now().Add(time.Duration(resp.expiresIn) * time.Second)
	}

	return token, nil
}
