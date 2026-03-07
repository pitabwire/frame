package implementation

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"

	"github.com/pitabwire/util"
	"golang.org/x/net/http2"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/server"
)

type defaultDriver struct {
	ctx        context.Context
	port       string
	httpServer *http.Server
	listener   net.Listener
	tlsCfg     *tls.Config
}

func NewDefaultDriver(
	ctx context.Context,
	cfg config.ConfigurationHTTPServer,
	h http.Handler,
	port string,
) server.Driver {
	return NewDefaultDriverWithTLS(ctx, cfg, h, port, nil)
}

func NewDefaultDriverWithTLS(
	ctx context.Context,
	cfg config.ConfigurationHTTPServer,
	h http.Handler,
	port string,
	tlsCfg *tls.Config,
) server.Driver {
	if cfg == nil {
		panic("config.ConfigurationHTTPServer is required")
	}

	return &defaultDriver{
		ctx:  ctx,
		port: port,
		httpServer: &http.Server{
			Addr:              port,
			Handler:           h,
			ReadTimeout:       cfg.HTTPReadTimeout(),
			ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout(),
			WriteTimeout:      cfg.HTTPWriteTimeout(),
			IdleTimeout:       cfg.HTTPIdleTimeout(),
			MaxHeaderBytes:    cfg.HTTPMaxHeaderBytes(),
			BaseContext: func(_ net.Listener) context.Context {
				return ctx
			},
		},
		tlsCfg: server.NormalizeTLSConfig(tlsCfg),
	}
}

func (dd *defaultDriver) Context() context.Context {
	return dd.ctx
}

func (dd *defaultDriver) getListener(ctx context.Context,
	address string, listener net.Listener) (net.Listener, error) {
	if listener != nil {
		return listener, nil
	}

	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}

	if dd.tlsCfg == nil {
		return listener, nil
	}

	return tls.NewListener(listener, dd.tlsCfg), nil
}

func (dd *defaultDriver) configureProtocols(isTLS bool) error {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	if isTLS {
		protocols.SetHTTP2(true)
		dd.httpServer.Protocols = protocols
		return http2.ConfigureServer(dd.httpServer, nil)
	}

	protocols.SetUnencryptedHTTP2(true)
	dd.httpServer.Protocols = protocols
	return nil
}

// ListenAndServe sets the address and handlers on DefaultDriver's http.Server,
// then calls ListenAndServe on it.
func (dd *defaultDriver) ListenAndServe(addr string, h http.Handler) error {
	var ln net.Listener

	dd.httpServer.Addr = addr
	dd.httpServer.Handler = h
	if err := dd.configureProtocols(dd.tlsCfg != nil); err != nil {
		return err
	}

	logField := "http port"
	if dd.tlsCfg != nil {
		logField = "https port"
	}
	log := util.Log(dd.ctx).WithField(logField, addr)

	ln, err0 := dd.getListener(dd.ctx, addr, dd.listener)
	if err0 != nil {
		return err0
	}

	log.Info("listening on server port")

	return dd.httpServer.Serve(ln)
}

func (dd *defaultDriver) ListenAndServeTLS(addr, certPath, certKeyPath string, h http.Handler) error {
	dd.httpServer.Addr = addr
	dd.httpServer.Handler = h
	log := util.Log(dd.ctx).WithField("https port", addr)

	if dd.tlsCfg == nil {
		var err error
		dd.tlsCfg, err = server.TLSConfigFromPath(certPath, certKeyPath)
		if err != nil {
			return err
		}
	}

	dd.tlsCfg = server.NormalizeTLSConfig(dd.tlsCfg)
	if err := dd.configureProtocols(true); err != nil {
		return err
	}

	ln, err0 := dd.getListener(dd.ctx, addr, dd.listener)
	if err0 != nil {
		return err0
	}

	log.Info("listening on server port")
	return dd.httpServer.Serve(ln)
}

func (dd *defaultDriver) Shutdown(ctx context.Context) error {
	return dd.httpServer.Shutdown(ctx)
}
