package client

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/xid"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"golang.org/x/oauth2"

	"github.com/pitabwire/frame/config"
)

const (
	defaultPrivateKeyJWTAssertionTTL = 5 * time.Minute
	maxTokenEndpointErrorBodyBytes   = 8 << 10
	//nolint:gosec // standards-defined identifier, not a credential
	clientAssertionTypeJWTBearer = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
)

//nolint:gochecknoglobals // test hook for Workload API fetch behavior
var fetchPrivateKeyJWTSVIDs = workloadapi.FetchX509SVIDs

type privateKeyJWTTokenSource struct {
	ctx          context.Context
	httpClient   *http.Client
	tokenURL     string
	clientID     string
	audiences    []string
	privateJWT   *config.PrivateKeyJWTConfig
	now          func() time.Time
	assertionTTL time.Duration
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

func resolveAutoOAuth2TokenSource(
	ctx context.Context,
	httpClient *http.Client,
) (oauth2.TokenSource, bool, error) {
	cfgAny := config.FromContext[any](ctx)
	oauthCfg, ok := cfgAny.(config.ConfigurationOAUTH2)
	if !ok {
		return nil, false, nil
	}

	privateJWT, enabled := autoPrivateKeyJWTConfig(oauthCfg)
	if !enabled {
		return nil, false, nil
	}

	tokenURL, err := autoPrivateKeyJWTTokenURL(ctx, oauthCfg)
	if err != nil {
		return nil, true, err
	}

	clientID := strings.TrimSpace(oauthCfg.GetOauth2ServiceClientID())
	switch {
	case clientID == "":
		return nil, true, errors.New("oauth2 private_key_jwt requires OAUTH2_SERVICE_CLIENT_ID")
	case tokenURL == "":
		return nil, true, errors.New("oauth2 private_key_jwt requires a token endpoint")
	case httpClient == nil:
		return nil, true, errors.New("oauth2 private_key_jwt requires an HTTP client")
	}

	return &privateKeyJWTTokenSource{
		ctx:          ctx,
		httpClient:   httpClient,
		tokenURL:     tokenURL,
		clientID:     clientID,
		audiences:    append([]string(nil), oauthCfg.GetOauth2ServiceAudience()...),
		privateJWT:   privateJWT,
		now:          time.Now,
		assertionTTL: defaultPrivateKeyJWTAssertionTTL,
	}, true, nil
}

func autoPrivateKeyJWTConfig(oauthCfg config.ConfigurationOAUTH2) (*config.PrivateKeyJWTConfig, bool) {
	if oauthCfg == nil {
		return nil, false
	}

	method := strings.TrimSpace(oauthCfg.GetOauth2TokenEndpointAuthMethod())
	if method != config.TokenEndpointAuthMethodPrivateKeyJWT {
		return nil, false
	}

	privateJWT := oauthCfg.GetOauth2PrivateKeyJWTConfig()
	if !usesWorkloadAPIPrivateKeyJWT(privateJWT) {
		return nil, false
	}

	return privateJWT, true
}

func autoPrivateKeyJWTTokenURL(
	ctx context.Context,
	oauthCfg config.ConfigurationOAUTH2,
) (string, error) {
	tokenURL := strings.TrimSpace(oauthCfg.GetOauth2TokenEndpoint())
	if tokenURL != "" || strings.TrimSpace(oauthCfg.GetOauth2ServiceURI()) == "" {
		return tokenURL, nil
	}

	if err := oauthCfg.LoadOauth2Config(ctx); err != nil {
		return "", fmt.Errorf("load oauth2 discovery for private_key_jwt: %w", err)
	}

	return strings.TrimSpace(oauthCfg.GetOauth2TokenEndpoint()), nil
}

func (s *privateKeyJWTTokenSource) Token() (*oauth2.Token, error) {
	if s == nil {
		return nil, errors.New("private_key_jwt token source is required")
	}

	form, err := s.tokenEndpointForm()
	if err != nil {
		return nil, err
	}

	statusCode, body, err := s.doTokenRequest(form)
	if err != nil {
		return nil, err
	}

	tokenResp, err := decodeTokenEndpointResponse(statusCode, body)
	if err != nil {
		return nil, err
	}

	return s.oauth2Token(tokenResp)
}

func (s *privateKeyJWTTokenSource) tokenEndpointForm() (url.Values, error) {
	assertion, err := s.clientAssertion()
	if err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", s.clientID)
	form.Set("client_assertion_type", clientAssertionTypeJWTBearer)
	form.Set("client_assertion", assertion)
	for _, audience := range s.audiences {
		trimmed := strings.TrimSpace(audience)
		if trimmed == "" {
			continue
		}
		form.Add("audience", trimmed)
	}

	return form, nil
}

func (s *privateKeyJWTTokenSource) doTokenRequest(form url.Values) (int, []byte, error) {
	req, err := http.NewRequestWithContext(
		s.ctx,
		http.MethodPost,
		s.tokenURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return 0, nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenEndpointErrorBodyBytes))
	if err != nil {
		return 0, nil, err
	}

	return resp.StatusCode, body, nil
}

func decodeTokenEndpointResponse(
	statusCode int,
	body []byte,
) (*tokenEndpointResponse, error) {
	var tokenResp tokenEndpointResponse
	if len(body) > 0 {
		parseErr := json.Unmarshal(body, &tokenResp)
		if parseErr != nil && statusCode >= http.StatusBadRequest {
			return nil, fmt.Errorf("oauth2 token endpoint returned status %d", statusCode)
		}
	}

	if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
		return &tokenResp, nil
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

func (s *privateKeyJWTTokenSource) oauth2Token(tokenResp *tokenEndpointResponse) (*oauth2.Token, error) {
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
		token.Expiry = s.now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	return token, nil
}

func (s *privateKeyJWTTokenSource) clientAssertion() (string, error) {
	svids, err := fetchPrivateKeyJWTSVIDs(s.ctx)
	if err != nil {
		return "", err
	}

	svid, err := selectPrivateKeyJWTSVID(
		svids,
		strings.TrimSpace(s.privateJWT.SPIFFEID),
		strings.TrimSpace(s.privateJWT.Hint),
	)
	if err != nil {
		return "", err
	}

	method, signer, err := signingMethodForPrivateKeyJWT(svid)
	if err != nil {
		return "", err
	}

	now := s.now().UTC()
	audience := strings.TrimSpace(s.privateJWT.Audience)
	if audience == "" {
		audience = s.tokenURL
	}

	issuer := strings.TrimSpace(s.privateJWT.Issuer)
	if issuer == "" {
		issuer = s.clientID
	}

	subject := strings.TrimSpace(s.privateJWT.Subject)
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

	token := jwt.NewWithClaims(method, claims)
	if keyID := strings.TrimSpace(s.privateJWT.KeyID); keyID != "" {
		token.Header["kid"] = keyID
	}

	return token.SignedString(signer)
}

func usesWorkloadAPIPrivateKeyJWT(cfg *config.PrivateKeyJWTConfig) bool {
	if cfg == nil || cfg.IsZero() {
		return false
	}

	source := strings.ToLower(strings.TrimSpace(cfg.Source))
	if source != "" {
		return source == config.PrivateKeyJWTSourceWorkloadAPI
	}

	return strings.TrimSpace(cfg.SPIFFEID) != "" || strings.TrimSpace(cfg.Hint) != ""
}

func selectPrivateKeyJWTSVID(
	svids []*x509svid.SVID,
	expectedSPIFFEID string,
	expectedHint string,
) (*x509svid.SVID, error) {
	if len(svids) == 0 {
		return nil, errors.New("workload API returned no X509-SVIDs")
	}

	expectedSPIFFEID = strings.TrimSpace(expectedSPIFFEID)
	expectedHint = strings.TrimSpace(expectedHint)

	if expectedSPIFFEID == "" && expectedHint == "" {
		return svids[0], nil
	}

	for _, svid := range svids {
		if svid == nil {
			continue
		}
		if expectedSPIFFEID != "" && svid.ID.String() != expectedSPIFFEID {
			continue
		}
		if expectedHint != "" && strings.TrimSpace(svid.Hint) != expectedHint {
			continue
		}

		return svid, nil
	}

	if expectedSPIFFEID != "" && expectedHint != "" {
		return nil, errors.New(
			"workload API did not return an X509-SVID matching the configured SPIFFE ID and hint",
		)
	}
	if expectedSPIFFEID != "" {
		return nil, errors.New("workload API did not return an X509-SVID matching the configured SPIFFE ID")
	}

	return nil, errors.New("workload API did not return an X509-SVID matching the configured hint")
}

func signingMethodForPrivateKeyJWT(
	svid *x509svid.SVID,
) (jwt.SigningMethod, crypto.Signer, error) {
	if svid == nil || svid.PrivateKey == nil {
		return nil, nil, errors.New("workload API X509-SVID private key is required")
	}

	signer := svid.PrivateKey

	switch signer.(type) {
	case *rsa.PrivateKey:
		return jwt.SigningMethodRS256, signer, nil
	case *ecdsa.PrivateKey:
		return jwt.SigningMethodES256, signer, nil
	case ed25519.PrivateKey:
		return jwt.SigningMethodEdDSA, signer, nil
	default:
		return nil, nil, errors.New(
			"workload API X509-SVID private key type is unsupported for private_key_jwt",
		)
	}
}
