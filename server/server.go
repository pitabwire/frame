package server

import (
	"crypto/tls"
	"errors"

	"gocloud.dev/server/driver"
	"golang.org/x/net/http2"
)

type Driver interface {
	driver.Server
	driver.TLSServer
}

var ErrTLSPathsNotProvided = errors.New("TLS certificate path or key path not provided")

func TLSConfigFromPath(certPath, certKeyPath string) (*tls.Config, error) {
	if certPath == "" || certKeyPath == "" {
		return nil, ErrTLSPathsNotProvided
	}

	cert, err := tls.LoadX509KeyPair(certPath, certKeyPath)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{http2.NextProtoTLS, "http/1.1"},
		MinVersion:   tls.VersionTLS12,
	}, nil
}
