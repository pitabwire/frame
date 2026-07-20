package push_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/pitabwire/frame/v2/queue"
	"github.com/pitabwire/frame/v2/queue/push"
	"github.com/stretchr/testify/require"
)

func TestHTTPStatusFor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		code int
	}{
		{nil, http.StatusOK},
		{queue.ErrUnauthorized, http.StatusUnauthorized},
		{queue.ErrForbidden, http.StatusForbidden},
		{queue.ErrTooLarge, http.StatusRequestEntityTooLarge},
		{queue.ErrDecode, http.StatusBadRequest},
		{queue.ErrNotRetryable, http.StatusUnprocessableEntity},
		{context.DeadlineExceeded, http.StatusGatewayTimeout},
		{errors.New("boom"), http.StatusServiceUnavailable},
		{fmt.Errorf("%w: detail", queue.ErrNotRetryable), http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		require.Equal(t, tc.code, push.HTTPStatusFor(tc.err))
	}
}
