package frame

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-jose/go-jose/v4"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"

	"github.com/pitabwire/frame/config"
)

const defaultOAuth2ClientJWKSPath = "/.well-known/oauth2-client-jwks.json"

//nolint:gochecknoglobals // test hook for Workload API fetch behavior
var fetchOAuth2ClientJWKSSVIDs = workloadapi.FetchX509SVIDs

func (s *Service) registerOAuth2ClientJWKSRoute(mux *http.ServeMux) {
	if mux == nil {
		return
	}

	cfg, ok := s.Config().(config.ConfigurationOAUTH2)
	if !ok {
		return
	}

	privateKeyJWT := cfg.GetOauth2PrivateKeyJWTConfig()
	if !usesWorkloadAPIPrivateKeyJWT(privateKeyJWT) {
		return
	}

	mux.HandleFunc(defaultOAuth2ClientJWKSPath, s.handleOAuth2ClientJWKS)
}

func (s *Service) handleOAuth2ClientJWKS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	cfg, ok := s.Config().(config.ConfigurationOAUTH2)
	if !ok {
		http.NotFound(w, r)
		return
	}

	privateKeyJWT := cfg.GetOauth2PrivateKeyJWTConfig()
	if !usesWorkloadAPIPrivateKeyJWT(privateKeyJWT) {
		http.NotFound(w, r)
		return
	}

	jwks, err := buildOAuth2ClientJWKS(r.Context(), privateKeyJWT)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}

	_ = json.NewEncoder(w).Encode(jwks)
}

func buildOAuth2ClientJWKS(
	ctx context.Context,
	cfg *config.PrivateKeyJWTConfig,
) (*jose.JSONWebKeySet, error) {
	if !usesWorkloadAPIPrivateKeyJWT(cfg) {
		return nil, errors.New("oauth2 private_key_jwt workload API configuration is required")
	}

	svids, err := fetchOAuth2ClientJWKSSVIDs(ctx)
	if err != nil {
		return nil, err
	}

	svid, err := selectOAuth2ClientJWKSSVID(svids, strings.TrimSpace(cfg.SPIFFEID), strings.TrimSpace(cfg.Hint))
	if err != nil {
		return nil, err
	}

	publicKey, err := oauth2ClientJWKSPublicKey(svid)
	if err != nil {
		return nil, err
	}

	jwk := jose.JSONWebKey{
		Key:       publicKey,
		Use:       "sig",
		Algorithm: oauth2ClientJWKSAlgorithm(publicKey),
	}
	if keyID := strings.TrimSpace(cfg.KeyID); keyID != "" {
		jwk.KeyID = keyID
	}

	return &jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk}}, nil
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

func selectOAuth2ClientJWKSSVID(
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
		return nil, errors.New("workload API did not return an X509-SVID matching the configured SPIFFE ID and hint")
	}
	if expectedSPIFFEID != "" {
		return nil, errors.New("workload API did not return an X509-SVID matching the configured SPIFFE ID")
	}

	return nil, errors.New("workload API did not return an X509-SVID matching the configured hint")
}

func oauth2ClientJWKSPublicKey(svid *x509svid.SVID) (crypto.PublicKey, error) {
	if svid == nil {
		return nil, errors.New("workload API X509-SVID is required")
	}
	if len(svid.Certificates) > 0 && svid.Certificates[0] != nil && svid.Certificates[0].PublicKey != nil {
		return svid.Certificates[0].PublicKey, nil
	}
	if svid.PrivateKey != nil {
		return svid.PrivateKey.Public(), nil
	}

	return nil, errors.New("workload API X509-SVID did not include a usable public key")
}

func oauth2ClientJWKSAlgorithm(publicKey crypto.PublicKey) string {
	switch publicKey.(type) {
	case *rsa.PublicKey:
		return "RS256"
	case *ecdsa.PublicKey:
		return "ES256"
	case ed25519.PublicKey:
		return "EdDSA"
	default:
		return ""
	}
}
