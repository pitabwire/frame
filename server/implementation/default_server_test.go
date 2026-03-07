package implementation_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"golang.org/x/net/http2"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/server"
	"github.com/pitabwire/frame/server/implementation"
)

type DefaultDriverSuite struct {
	suite.Suite
	httpCfg *config.ConfigurationDefault
}

func TestDefaultDriverSuite(t *testing.T) {
	suite.Run(t, new(DefaultDriverSuite))
}

func (s *DefaultDriverSuite) SetupTest() {
	s.httpCfg = &config.ConfigurationDefault{}
}

func (s *DefaultDriverSuite) TestListenAndServeSupportsH2C() {
	addr := s.freeAddress()
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	driver := implementation.NewDefaultDriver(context.Background(), s.httpCfg, handler, addr)
	errCh := s.startDriver(func() error {
		return driver.ListenAndServe(addr, handler)
	})

	client := &http.Client{Transport: newH2CTransport()}
	resp := s.waitForResponse(client, "http://"+addr)
	defer resp.Body.Close()

	s.Equal(2, resp.ProtoMajor)
	s.Nil(resp.TLS)
	s.shutdownDriver(driver, errCh)
}

func (s *DefaultDriverSuite) TestListenAndServeTLSNegotiatesHTTP2() {
	addr := s.freeAddress()
	serverCert, _, caPEM, caPool := s.generateTLSMaterial(false)
	certPath, keyPath := s.writeKeyPair(serverCert)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	driver := implementation.NewDefaultDriver(context.Background(), s.httpCfg, handler, addr)
	errCh := s.startDriver(func() error {
		return driver.ListenAndServeTLS(addr, certPath, keyPath, handler)
	})

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    caPool,
				MinVersion: tls.VersionTLS12,
			},
			ForceAttemptHTTP2: true,
		},
	}

	resp := s.waitForResponse(client, "https://"+addr)
	defer resp.Body.Close()

	s.Equal(http.StatusAccepted, resp.StatusCode)
	s.Equal(2, resp.ProtoMajor)
	s.Require().NotNil(resp.TLS)
	s.Equal(http2.NextProtoTLS, resp.TLS.NegotiatedProtocol)
	s.NotEmpty(caPEM)

	s.shutdownDriver(driver, errCh)
}

func (s *DefaultDriverSuite) TestListenAndServeWithInjectedMutualTLSNegotiatesHTTP2() {
	addr := s.freeAddress()
	serverCert, clientCert, _, caPool := s.generateTLSMaterial(true)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	driverTLS := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
		MinVersion:   tls.VersionTLS12,
	}

	driver := implementation.NewDefaultDriverWithTLS(context.Background(), s.httpCfg, handler, addr, driverTLS)
	errCh := s.startDriver(func() error {
		return driver.ListenAndServe(addr, handler)
	})

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      caPool,
				Certificates: []tls.Certificate{clientCert},
				MinVersion:   tls.VersionTLS12,
			},
			ForceAttemptHTTP2: true,
		},
	}

	resp := s.waitForResponse(client, "https://"+addr)
	defer resp.Body.Close()

	s.Equal(http.StatusCreated, resp.StatusCode)
	s.Equal(2, resp.ProtoMajor)
	s.Require().NotNil(resp.TLS)
	s.Equal(http2.NextProtoTLS, resp.TLS.NegotiatedProtocol)

	plainClient := &http.Client{Transport: newH2CTransport()}
	_, err := plainClient.Get("http://" + addr)
	s.Require().Error(err)

	s.shutdownDriver(driver, errCh)
}

func newH2CTransport() *http2.Transport {
	return &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			dialer := &net.Dialer{}
			return dialer.DialContext(ctx, network, addr)
		},
	}
}

func (s *DefaultDriverSuite) freeAddress() string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	s.Require().NoError(err)
	defer listener.Close()
	return listener.Addr().String()
}

func (s *DefaultDriverSuite) startDriver(run func() error) chan error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- run()
	}()
	return errCh
}

func (s *DefaultDriverSuite) waitForResponse(client *http.Client, url string) *http.Response {
	s.T().Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := client.Get(url)
		if err == nil {
			return resp
		}
		if time.Now().After(deadline) {
			s.T().Fatalf("request to %s did not succeed before timeout: %v", url, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (s *DefaultDriverSuite) shutdownDriver(driver server.Driver, errCh chan error) {
	s.T().Helper()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.Require().NoError(driver.Shutdown(shutdownCtx))

	select {
	case err := <-errCh:
		s.True(err == nil || errors.Is(err, http.ErrServerClosed))
	case <-time.After(5 * time.Second):
		s.T().Fatal("server did not stop")
	}
}

func (s *DefaultDriverSuite) writeKeyPair(cert tls.Certificate) (string, string) {
	s.T().Helper()

	key, ok := cert.PrivateKey.(*rsa.PrivateKey)
	s.Require().True(ok)

	dir := s.T().TempDir()
	certPath := filepath.Join(dir, "server.pem")
	keyPath := filepath.Join(dir, "server-key.pem")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	s.Require().NoError(os.WriteFile(certPath, certPEM, 0o600))
	s.Require().NoError(os.WriteFile(keyPath, keyPEM, 0o600))

	return certPath, keyPath
}

func (s *DefaultDriverSuite) generateTLSMaterial(requireClientCert bool) (
	tls.Certificate,
	tls.Certificate,
	[]byte,
	*x509.CertPool,
) {
	s.T().Helper()

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	s.Require().NoError(err)

	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "frame-test-ca",
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	s.Require().NoError(err)

	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	caPool := x509.NewCertPool()
	s.Require().True(caPool.AppendCertsFromPEM(caPEM))

	serverCert := s.issueLeafCert(
		big.NewInt(2),
		"127.0.0.1",
		[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		caTemplate,
		caKey,
	)
	var clientCert tls.Certificate
	if requireClientCert {
		clientCert = s.issueLeafCert(
			big.NewInt(3),
			"frame-client",
			[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			caTemplate,
			caKey,
		)
	}

	return serverCert, clientCert, caPEM, caPool
}

func (s *DefaultDriverSuite) issueLeafCert(
	serial *big.Int,
	commonName string,
	keyUsage []x509.ExtKeyUsage,
	caCert *x509.Certificate,
	caKey *rsa.PrivateKey,
) tls.Certificate {
	s.T().Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	s.Require().NoError(err)

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: keyUsage,
	}

	if commonName == "127.0.0.1" {
		template.IPAddresses = []net.IP{net.ParseIP(commonName)}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	s.Require().NoError(err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	s.Require().NoError(err)
	return cert
}
