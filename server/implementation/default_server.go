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
	httpCfg config.ConfigurationHTTPServer,
	h http.Handler,
	port string,
) server.Driver {
	return NewDefaultDriverWithTLS(ctx, httpCfg, h, port, nil)
}

func NewDefaultDriverWithTLS(
	ctx context.Context,
	httpCfg config.ConfigurationHTTPServer,
	h http.Handler,
	port string,
	tlsCfg *tls.Config,
) server.Driver {
	if httpCfg == nil {
		panic("config.ConfigurationHTTPServer is required")
	}

	return &defaultDriver{
		ctx:  ctx,
		port: port,
		httpServer: &http.Server{
			Addr:              port,
			Handler:           h,
			ReadTimeout:       httpCfg.HTTPReadTimeout(),
			ReadHeaderTimeout: httpCfg.HTTPReadHeaderTimeout(),
			WriteTimeout:      httpCfg.HTTPWriteTimeout(),
			IdleTimeout:       httpCfg.HTTPIdleTimeout(),
			MaxHeaderBytes:    httpCfg.HTTPMaxHeaderBytes(),
			BaseContext: func(_ net.Listener) context.Context {
				return ctx
			},
		},
		tlsCfg: tlsCfg,
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

// ListenAndServe sets the address and handlers on DefaultDriver's http.Server,
// then calls ListenAndServe on it.
func (dd *defaultDriver) ListenAndServe(addr string, h http.Handler) error {
	var ln net.Listener

	dd.httpServer.Addr = addr
	dd.httpServer.Handler = h
	log := util.Log(dd.ctx).WithField("http port", addr)

	// Configure h2c (HTTP/2 without TLS) by default
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	dd.httpServer.Protocols = protocols

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

	// Configure standard HTTP/2 with TLS when h2c is explicitly disabled
	err := http2.ConfigureServer(dd.httpServer, nil)
	if err != nil {
		return err
	}

	if dd.tlsCfg == nil {
		dd.tlsCfg, err = server.TLSConfigFromPath(certPath, certKeyPath)
		if err != nil {
			return err
		}
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
