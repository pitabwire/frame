package client //nolint:testpackage // tests access unexported newTeeReadCloser

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

type loggingRoundTripFunc func(*http.Request) (*http.Response, error)

func (f loggingRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type LoggingTransportSuite struct {
	suite.Suite
}

func TestLoggingTransportSuite(t *testing.T) {
	suite.Run(t, new(LoggingTransportSuite))
}

func (s *LoggingTransportSuite) TestTeeReadCloser() {
	original := io.NopCloser(strings.NewReader("hello world"))
	tr := newTeeReadCloser(original, 5)

	buf, err := io.ReadAll(tr)
	s.Require().NoError(err)
	s.Equal("hello world", string(buf))
	s.Equal("hello", string(tr.LoggedBody()))
	s.NoError(tr.Close())
}

func (s *LoggingTransportSuite) TestNewLoggingTransportAndRoundTrip() {
	base := loggingRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		_, _ = io.ReadAll(req.Body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString("response body")),
			Header:     http.Header{"X-Test": []string{"yes"}},
		}, nil
	})

	rt := NewLoggingTransport(base,
		WithTransportLogRequests(true),
		WithTransportLogResponses(true),
		WithTransportLogHeaders(true),
		WithTransportLogBody(true),
		WithTransportMaxBodySize(10),
	)

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"http://example.com",
		io.NopCloser(strings.NewReader("request body payload")),
	)
	s.Require().NoError(err)
	resp, err := rt.RoundTrip(req)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)
	s.NoError(resp.Body.Close())
}

func (s *LoggingTransportSuite) TestRoundTripErrorAndWrapClient() {
	base := loggingRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})
	rt := NewLoggingTransport(base,
		WithTransportLogRequests(true),
		WithTransportLogResponses(true),
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	s.Require().NoError(err)
	resp, err := rt.RoundTrip(req)
	s.Nil(resp)
	s.Require().Error(err)

	wrapped := WrapClient(nil, WithTransportLogRequests(true))
	s.NotNil(wrapped)
	s.NotNil(wrapped.Transport)
}
