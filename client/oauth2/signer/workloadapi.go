package signer

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"errors"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"

	"github.com/pitabwire/frame/config"
)

//nolint:gochecknoglobals // test hook for Workload API fetch behavior
var fetchX509SVIDs = workloadapi.FetchX509SVIDs

// SetFetchX509SVIDsForTest replaces the Workload API fetch function and
// returns a cleanup function that restores the original.
func SetFetchX509SVIDsForTest(
	fn func(context.Context, ...workloadapi.ClientOption) ([]*x509svid.SVID, error),
) func() {
	orig := fetchX509SVIDs
	fetchX509SVIDs = fn
	return func() { fetchX509SVIDs = orig }
}

// WorkloadAPISigner signs JWT assertions using SPIFFE X509-SVIDs fetched
// from the Workload API.
type WorkloadAPISigner struct {
	spiffeID string
	hint     string
	keyID    string
}

// NewWorkloadAPISigner creates a signer backed by the SPIFFE Workload API.
func NewWorkloadAPISigner(cfg *config.PrivateKeyJWTConfig) *WorkloadAPISigner {
	return &WorkloadAPISigner{
		spiffeID: strings.TrimSpace(cfg.SPIFFEID),
		hint:     strings.TrimSpace(cfg.Hint),
		keyID:    strings.TrimSpace(cfg.KeyID),
	}
}

func (s *WorkloadAPISigner) Algorithm(ctx context.Context) (string, error) {
	svid, err := s.fetchSVID(ctx)
	if err != nil {
		return "", err
	}

	method, _, err := signingMethodForKey(svid.PrivateKey)
	if err != nil {
		return "", err
	}

	return method.Alg(), nil
}

func (s *WorkloadAPISigner) KeyID(_ context.Context) (string, error) {
	return s.keyID, nil
}

func (s *WorkloadAPISigner) Sign(ctx context.Context, payload []byte) ([]byte, error) {
	svid, err := s.fetchSVID(ctx)
	if err != nil {
		return nil, err
	}

	method, key, err := signingMethodForKey(svid.PrivateKey)
	if err != nil {
		return nil, err
	}

	return method.Sign(string(payload), key)
}

func (s *WorkloadAPISigner) fetchSVID(ctx context.Context) (*x509svid.SVID, error) {
	svids, err := fetchX509SVIDs(ctx)
	if err != nil {
		return nil, err
	}

	return selectSVID(svids, s.spiffeID, s.hint)
}

func selectSVID(
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

func signingMethodForKey(
	key crypto.Signer,
) (jwt.SigningMethod, crypto.Signer, error) {
	if key == nil {
		return nil, nil, errors.New("workload API X509-SVID private key is required")
	}

	switch key.(type) {
	case *rsa.PrivateKey:
		return jwt.SigningMethodRS256, key, nil
	case *ecdsa.PrivateKey:
		return jwt.SigningMethodES256, key, nil
	case ed25519.PrivateKey:
		return jwt.SigningMethodEdDSA, key, nil
	default:
		return nil, nil, errors.New(
			"workload API X509-SVID private key type is unsupported for private_key_jwt",
		)
	}
}
