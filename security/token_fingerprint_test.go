package security_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/pitabwire/frame/v3/security"
	"github.com/stretchr/testify/require"
)

func TestTokenFingerprint(t *testing.T) {
	t.Parallel()

	sum := sha256.Sum256([]byte("abc.def.ghi"))
	cases := []struct {
		name  string
		token string
		want  string
	}{
		{name: "first 8 hex of sha256", token: "abc.def.ghi", want: hex.EncodeToString(sum[:])[:8]},
		{name: "empty token", token: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, security.TokenFingerprint(tc.token))
		})
	}
}
