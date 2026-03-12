package oauth2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2"

	"github.com/pitabwire/frame/client/oauth2/auth"
	"github.com/pitabwire/frame/config"
)

const (
	authMethodClientSecretBasic = "client_secret_basic"

	authMethodClientSecretPost = "client_secret_post"
)

// NewTokenSource inspects the OAuth2 configuration and returns the
// appropriate oauth2.TokenSource for the configured auth method.
//
// Supported methods:
//   - client_secret_basic (default)
//   - client_secret_post
//   - private_key_jwt
func NewTokenSource(
	ctx context.Context,
	httpClient *http.Client,
	cfg config.ConfigurationOAUTH2,
) (oauth2.TokenSource, error) {
	if cfg == nil {
		return nil, errors.New("oauth2 config is required")
	}
	if httpClient == nil {
		return nil, errors.New("oauth2 HTTP client is required")
	}

	tokenURL, err := auth.ResolveTokenEndpoint(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if tokenURL == "" {
		return nil, errors.New("oauth2 token endpoint is required")
	}

	method := strings.TrimSpace(cfg.GetOauth2TokenEndpointAuthMethod())
	if method == "" {
		return nil, errors.New("oauth2 token_endpoint_auth_method is required")
	}

	switch method {
	case authMethodClientSecretBasic:
		return auth.NewBasicTokenSource(ctx, httpClient, cfg, tokenURL)
	case authMethodClientSecretPost:
		return auth.NewPostTokenSource(ctx, httpClient, cfg, tokenURL)
	case config.TokenEndpointAuthMethodPrivateKeyJWT:
		return newPrivateKeyJWTSource(ctx, httpClient, cfg, tokenURL)
	default:
		return nil, fmt.Errorf(
			"unsupported oauth2 token endpoint auth method %q",
			method,
		)
	}
}

func newPrivateKeyJWTSource(
	ctx context.Context,
	httpClient *http.Client,
	cfg config.ConfigurationOAUTH2,
	tokenURL string,
) (oauth2.TokenSource, error) {
	pkcfg := cfg.GetOauth2PrivateKeyJWTConfig()
	if pkcfg != nil && strings.ToLower(strings.TrimSpace(pkcfg.Source)) == config.PrivateKeyJWTSourceURL {
		return auth.NewRemoteSignerTokenSource(ctx, httpClient, cfg, tokenURL)
	}
	return auth.NewPrivateKeyJWTTokenSource(ctx, httpClient, cfg, tokenURL)
}
