package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/pitabwire/frame/config"
)

// Style selects how client credentials are sent to the token endpoint.
type Style int

const (
	// StyleBasic sends client_id/client_secret as HTTP Basic auth.
	StyleBasic Style = iota
	// StylePost sends client_id/client_secret in the POST body.
	StylePost
)

type clientSecretTokenSource struct {
	ctx          context.Context
	httpClient   *http.Client
	tokenURL     string
	clientID     string
	clientSecret string
	audiences    []string
	authStyle    Style
	now          func() time.Time
}

// NewBasicTokenSource returns an oauth2.TokenSource that authenticates with
// client_secret_basic (HTTP Basic auth).
func NewBasicTokenSource(
	ctx context.Context,
	httpClient *http.Client,
	cfg config.ConfigurationOAUTH2,
	tokenURL string,
) (oauth2.TokenSource, error) {
	return newClientSecretTokenSource(ctx, httpClient, cfg, tokenURL, StyleBasic)
}

// NewPostTokenSource returns an oauth2.TokenSource that authenticates with
// client_secret_post (credentials in the POST body).
func NewPostTokenSource(
	ctx context.Context,
	httpClient *http.Client,
	cfg config.ConfigurationOAUTH2,
	tokenURL string,
) (oauth2.TokenSource, error) {
	return newClientSecretTokenSource(ctx, httpClient, cfg, tokenURL, StylePost)
}

func newClientSecretTokenSource(
	ctx context.Context,
	httpClient *http.Client,
	cfg config.ConfigurationOAUTH2,
	tokenURL string,
	style Style,
) (oauth2.TokenSource, error) {
	clientID := strings.TrimSpace(cfg.GetOauth2ServiceClientID())
	if clientID == "" {
		return nil, errors.New("client_secret auth requires client ID")
	}

	clientSecret := strings.TrimSpace(cfg.GetOauth2ServiceClientSecret())
	if clientSecret == "" {
		return nil, errors.New("client_secret auth requires client secret")
	}

	return &clientSecretTokenSource{
		ctx:          ctx,
		httpClient:   httpClient,
		tokenURL:     tokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		audiences:    append([]string(nil), cfg.GetOauth2ServiceAudience()...),
		authStyle:    style,
		now:          time.Now,
	}, nil
}

func (s *clientSecretTokenSource) Token() (*oauth2.Token, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")

	var ba *basicAuth

	switch s.authStyle {
	case StyleBasic:
		ba = &basicAuth{
			username: s.clientID,
			password: s.clientSecret,
		}
	case StylePost:
		form.Set("client_id", s.clientID)
		form.Set("client_secret", s.clientSecret)
	default:
		return nil, errors.New("unsupported client secret auth style")
	}

	for _, aud := range s.audiences {
		aud = strings.TrimSpace(aud)
		if aud != "" {
			form.Add("audience", aud)
		}
	}

	return exchangeToken(
		s.ctx,
		s.httpClient,
		s.tokenURL,
		tokenEndpointRequest{
			form:      form,
			basicAuth: ba,
		},
		s.now,
	)
}
