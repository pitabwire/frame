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

// BasicAuth carries HTTP Basic credentials for the token endpoint.
type BasicAuth struct {
	Username string
	Password string //nolint:gosec // HTTP Basic auth field, not a hardcoded credential
}

// TokenEndpointRequest describes a POST to the token endpoint.
type TokenEndpointRequest struct {
	Form      url.Values
	BasicAuth *BasicAuth
}

type tokenEndpointResponse struct {
	//nolint:gosec // OAuth2 response payload field
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int64  `json:"expires_in"`
	Scope            string `json:"scope"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
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
func ExchangeToken(
	ctx context.Context,
	httpClient *http.Client,
	tokenURL string,
	req TokenEndpointRequest,
	now func() time.Time,
) (*oauth2.Token, error) {
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		tokenURL,
		strings.NewReader(req.Form.Encode()),
	)
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Accept", "application/json")

	if req.BasicAuth != nil {
		httpReq.SetBasicAuth(req.BasicAuth.Username, req.BasicAuth.Password)
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
	var tokenResp tokenEndpointResponse
	if len(body) > 0 {
		parseErr := json.Unmarshal(body, &tokenResp)
		if parseErr != nil && statusCode >= http.StatusBadRequest {
			return nil, fmt.Errorf("oauth2 token endpoint returned status %d", statusCode)
		}
	}

	if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
		return toOAuth2Token(&tokenResp, now)
	}

	if tokenResp.Error != "" {
		if tokenResp.ErrorDescription != "" {
			return nil, fmt.Errorf(
				"oauth2 token endpoint error: %s: %s",
				tokenResp.Error,
				tokenResp.ErrorDescription,
			)
		}
		return nil, fmt.Errorf("oauth2 token endpoint error: %s", tokenResp.Error)
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

func toOAuth2Token(
	tokenResp *tokenEndpointResponse,
	now func() time.Time,
) (*oauth2.Token, error) {
	if tokenResp == nil {
		return nil, errors.New("oauth2 token endpoint response is required")
	}
	if strings.TrimSpace(tokenResp.AccessToken) == "" {
		return nil, errors.New("oauth2 token endpoint response missing access_token")
	}

	tokenType := strings.TrimSpace(tokenResp.TokenType)
	if tokenType == "" {
		tokenType = "Bearer"
	}

	token := &oauth2.Token{
		AccessToken: tokenResp.AccessToken,
		TokenType:   tokenType,
	}
	if tokenResp.ExpiresIn > 0 {
		token.Expiry = now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	return token, nil
}
