package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/xid"
	"golang.org/x/oauth2"

	"github.com/pitabwire/frame/client/oauth2/signer"
	"github.com/pitabwire/frame/config"
)

const (
	defaultAssertionTTL = 5 * time.Minute
	//nolint:gosec // standards-defined identifier, not a credential
	clientAssertionTypeJWTBearer = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
)

type privateKeyJWTTokenSource struct {
	ctx          context.Context
	httpClient   *http.Client
	tokenURL     string
	clientID     string
	audiences    []string
	config       *config.PrivateKeyJWTConfig
	signer       signer.JWTAssertionSigner
	now          func() time.Time
	assertionTTL time.Duration
}

// NewPrivateKeyJWTTokenSource returns an oauth2.TokenSource that
// authenticates using the private_key_jwt method (RFC 7523).
func NewPrivateKeyJWTTokenSource(
	ctx context.Context,
	httpClient *http.Client,
	cfg config.ConfigurationOAUTH2,
	tokenURL string,
) (oauth2.TokenSource, error) {
	clientID := strings.TrimSpace(cfg.GetOauth2ServiceClientID())
	if clientID == "" {
		return nil, errors.New("private_key_jwt requires client ID")
	}

	pkcfg := cfg.GetOauth2PrivateKeyJWTConfig()
	if pkcfg == nil || pkcfg.IsZero() {
		return nil, errors.New("private_key_jwt config is required")
	}

	jwtSigner, err := signer.Resolve(ctx, httpClient, pkcfg)
	if err != nil {
		return nil, err
	}

	return &privateKeyJWTTokenSource{
		ctx:          ctx,
		httpClient:   httpClient,
		tokenURL:     tokenURL,
		clientID:     clientID,
		audiences:    append([]string(nil), cfg.GetOauth2ServiceAudience()...),
		config:       pkcfg,
		signer:       jwtSigner,
		now:          time.Now,
		assertionTTL: defaultAssertionTTL,
	}, nil
}

func (s *privateKeyJWTTokenSource) Token() (*oauth2.Token, error) {
	assertion, err := s.clientAssertion()
	if err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", s.clientID)
	form.Set("client_assertion_type", clientAssertionTypeJWTBearer)
	form.Set("client_assertion", assertion)

	for _, aud := range s.audiences {
		aud = strings.TrimSpace(aud)
		if aud != "" {
			form.Add("audience", aud)
		}
	}

	return ExchangeToken(
		s.ctx,
		s.httpClient,
		s.tokenURL,
		TokenEndpointRequest{Form: form},
		s.now,
	)
}

func (s *privateKeyJWTTokenSource) clientAssertion() (string, error) {
	now := s.now().UTC()

	audience := strings.TrimSpace(s.config.Audience)
	if audience == "" {
		audience = s.tokenURL
	}

	issuer := strings.TrimSpace(s.config.Issuer)
	if issuer == "" {
		issuer = s.clientID
	}

	subject := strings.TrimSpace(s.config.Subject)
	if subject == "" {
		subject = s.clientID
	}

	claims := jwt.RegisteredClaims{
		Issuer:    issuer,
		Subject:   subject,
		Audience:  jwt.ClaimStrings{audience},
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now.Add(-time.Minute)),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.assertionTTL)),
		ID:        xid.New().String(),
	}

	alg, err := s.signer.Algorithm(s.ctx)
	if err != nil {
		return "", fmt.Errorf("resolve signing algorithm: %w", err)
	}

	method := jwt.GetSigningMethod(alg)
	if method == nil {
		return "", fmt.Errorf("unsupported JWT signing algorithm: %s", alg)
	}

	token := jwt.NewWithClaims(method, claims)

	keyID, err := s.signer.KeyID(s.ctx)
	if err != nil {
		return "", fmt.Errorf("resolve key ID: %w", err)
	}
	if keyID != "" {
		token.Header["kid"] = keyID
	}

	// Build the "header.payload" signing input and delegate to the signer.
	ss, err := token.SigningString()
	if err != nil {
		return "", fmt.Errorf("build JWT signing input: %w", err)
	}

	sig, err := s.signer.Sign(s.ctx, []byte(ss))
	if err != nil {
		return "", err
	}

	return ss + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}
