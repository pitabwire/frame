package signer

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/pitabwire/frame/config"
)

// JWTAssertionSigner signs JWT client assertions for the private_key_jwt
// authentication method.
type JWTAssertionSigner interface {
	Algorithm(ctx context.Context) (string, error)
	KeyID(ctx context.Context) (string, error)
	Sign(ctx context.Context, payload []byte) ([]byte, error)
}

// Resolve inspects the private key JWT configuration and returns an
// appropriate signer implementation.
func Resolve(
	_ context.Context,
	httpClient *http.Client,
	cfg *config.PrivateKeyJWTConfig,
) (JWTAssertionSigner, error) {
	if cfg == nil || cfg.IsZero() {
		return nil, errors.New("private_key_jwt config is required")
	}

	source := strings.ToLower(strings.TrimSpace(cfg.Source))

	switch {
	case source == config.PrivateKeyJWTSourceURL:
		if strings.TrimSpace(cfg.SignerURL) == "" {
			return nil, errors.New("private_key_jwt url source requires signer_url")
		}
		return NewJWKSURLSigner(httpClient, cfg), nil
	case source == config.PrivateKeyJWTSourceWorkloadAPI:
		return NewWorkloadAPISigner(cfg), nil
	case strings.TrimSpace(cfg.SPIFFEID) != "" || strings.TrimSpace(cfg.Hint) != "":
		return NewWorkloadAPISigner(cfg), nil
	default:
		return nil, errors.New(
			"unsupported private_key_jwt signer source: must be url, workload_api, or provide SPIFFE ID/hint",
		)
	}
}
