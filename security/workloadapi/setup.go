package workloadapi

import (
	"context"
	"crypto/tls"
	"errors"

	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/workloadapi"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/security"
)

type workloadAPI struct {
	trustedDomain string
	source        *workloadapi.X509Source
}

func NewWorkloadAPI(cfg config.ConfigurationWorkloadAPI) security.WorkloadAPI {
	return &workloadAPI{trustedDomain: cfg.GetTrustedDomain()}
}

func (w *workloadAPI) Setup(ctx context.Context) (*tls.Config, error) {
	if w.trustedDomain == "" {
		return nil, errors.New("no trust domain set up for workload API")
	}

	td, err := spiffeid.TrustDomainFromString(w.trustedDomain)
	if err != nil {
		return nil, err
	}

	if w.source != nil {
		_ = w.source.Close()
		w.source = nil
	}

	// Connect to the SPIFFE Workload API exposed by the local SPIRE agent.
	source, err := workloadapi.NewX509Source(ctx)
	if err != nil {
		return nil, err
	}
	w.source = source

	// Only allow callers from our trust domain, or tighten this to an exact ID.
	authorizer := tlsconfig.AuthorizeMemberOf(td)

	tlsConfig := tlsconfig.MTLSServerConfig(source, source, authorizer)
	return tlsConfig, nil
}

func (w *workloadAPI) Close() {
	if w.source != nil {
		_ = w.source.Close()
	}
}
