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

const maxSignerResponseBytes = 64 << 10 // 64 KiB

//nolint:gosec // standards-defined identifier, not a credential
const remoteClientAssertionTypeJWTBearer = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"

type remoteSignerTokenSource struct {
	ctx        context.Context
	httpClient *http.Client
	tokenURL   string
	signerURL  string
	clientID   string
	audiences  []string
	audience   string
	now        func() time.Time
}

type signAssertionRequest struct {
	ClientID      string `json:"client_id"`
	TokenEndpoint string `json:"token_endpoint"`
	Audience      string `json:"audience"`
	JWKSetName    string `json:"jwk_set_name"`
}

type signAssertionResponse struct {
	ClientAssertion     string `json:"client_assertion"`
	ClientAssertionType string `json:"client_assertion_type"`
}

// NewRemoteSignerTokenSource returns an oauth2.TokenSource that authenticates
// using a remote signing service for private_key_jwt (RFC 7523). It calls
// the signing endpoint to get a signed JWT client assertion, then exchanges
// it at the token endpoint.
func NewRemoteSignerTokenSource(
	ctx context.Context,
	httpClient *http.Client,
	cfg config.ConfigurationOAUTH2,
	tokenURL string,
) (oauth2.TokenSource, error) {
	clientID := strings.TrimSpace(cfg.GetOauth2ServiceClientID())
	if clientID == "" {
		return nil, errors.New("remote signer requires client ID")
	}

	pkcfg := cfg.GetOauth2PrivateKeyJWTConfig()
	if pkcfg == nil {
		return nil, errors.New("remote signer requires private_key_jwt config")
	}

	signerURL := strings.TrimSpace(pkcfg.SignerURL)
	if signerURL == "" {
		return nil, errors.New("remote signer requires signer_url")
	}

	audience := strings.TrimSpace(pkcfg.Audience)
	if audience == "" {
		audience = tokenURL
	}

	return &remoteSignerTokenSource{
		ctx:        ctx,
		httpClient: httpClient,
		tokenURL:   tokenURL,
		signerURL:  signerURL,
		clientID:   clientID,
		audiences:  append([]string(nil), cfg.GetOauth2ServiceAudience()...),
		audience:   audience,
		now:        time.Now,
	}, nil
}

func (s *remoteSignerTokenSource) Token() (*oauth2.Token, error) {
	assertion, err := s.fetchSignedAssertion()
	if err != nil {
		return nil, fmt.Errorf("fetch signed assertion: %w", err)
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", s.clientID)
	form.Set("client_assertion_type", remoteClientAssertionTypeJWTBearer)
	form.Set("client_assertion", assertion)

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
		tokenEndpointRequest{form: form},
		s.now,
	)
}

func (s *remoteSignerTokenSource) fetchSignedAssertion() (string, error) {
	reqBody := signAssertionRequest{
		ClientID:      s.clientID,
		TokenEndpoint: s.tokenURL,
		Audience:      s.audience,
		JWKSetName:    s.clientID,
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal sign request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		s.ctx,
		http.MethodPost,
		s.signerURL,
		strings.NewReader(string(bodyJSON)),
	)
	if err != nil {
		return "", fmt.Errorf("build sign request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("call signer endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSignerResponseBytes))
	if err != nil {
		return "", fmt.Errorf("read signer response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"signer endpoint returned status %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	var signResp signAssertionResponse
	if parseErr := json.Unmarshal(body, &signResp); parseErr != nil {
		return "", fmt.Errorf("parse signer response: %w", parseErr)
	}

	if signResp.ClientAssertion == "" {
		return "", errors.New("signer endpoint returned empty client_assertion")
	}

	return signResp.ClientAssertion, nil
}
