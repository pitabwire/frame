package frame

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
)

const sampleAccessKey = "eyJhbGciOiJSUzI1NiIsImtpZCI6InB1YmxpYzpmODg2ZDBmNy0zYmY0LTQzMzgtOGU4Yy01ZjhjNmVlNGM3MWQiLCJ0eXAiOiJKV1QifQ.eyJhdF9oYXNoIjoiMHc1d3JUU0E4RDdUMFRFbU5lanNGUSIsImF1ZCI6WyJjMmY0ajdhdTZzN2Y5MXVxbm9rZyJdLCJhdXRoX3RpbWUiOjE2MjIyNDE2MDMsImV4cCI6MTYyMjI0NTIwOCwiaWF0IjoxNjIyMjQxNjA4LCJpc3MiOiJodHRwOi8vMTI3LjAuMC4xOjQ0NDQvIiwianRpIjoiZDBjNTQwODMtYzhhOC00NWZlLTk1NDQtYzY2NDAxN2IzM2M5Iiwibm9uY2UiOiIiLCJyYXQiOjE2MjIyNDE1NzMsInNpZCI6ImNjMTA1OWIyLWUzYzEtNGZhNS1iMTg4LWI0ZDY4MjQ1NDI5NyIsInN1YiI6ImMyb2hoYzNuZGJtMGI2Y2g5dGUwIn0.MEmkaztQt7MpershW8mqDKIJho26sd2M-7y-Fwx8nEmv7wm0L4LfHEqe5rpdOS4WfWVo6jPbTi_AdKOoYXzevD-q6q9-cGo7cFMLpzl5sjAyk6-HiULLkU1AobgjNOYw4oHhnsRwwMRLhYyUiYbA0uPfVycMTS182tsBZ8Lm7ctMTBepslWNzY-SwowVDrQOXnbkXynf31VY45u1Zl1DwggCW7z3B70iGLjLNY-YDy2Vl78ES2bOaD4G8V7Kn5NNua6s73bYgb8Yfadm9gzk9E6VAYJpjASAsMtjpNaArHrFYQJ7HIdYGnWwxSSKlWPA_YzKxd0QQzR5qEKJs_t8wY3EZjX79OcuvkZGoqASpupBrVPxyHHtTtvQWGtQRjoSojJsNxD3GWxzCPJnETwlTcMzBj6uY3CgAAHQCjdfbS3bodJrW-zbrGwyuG145m0maktnYH1gOqEaz22Gz8utFPWvDaq_cFNqkkazo8FUrRzwsd7crG1Z-zY6q0Jj6YvItzFUd5_hk_io-uhVyu3D_igeJsdNyXlBM3r-1FYmRwhcEu0sKX1S4jLs0VLF_UR4bOTbU4DgfSy4jtLRbdWb09HzkcMJvbwnh4QLiBf9Uu1WeaG2OIkYs5eDdZQPwAFLBRmupsRRZiAr-xxeZ0_0TDZPJYBU_8xoU5TIveG-8Cw"

func TestAuthenticationFromContext(t *testing.T) {
	ctx := context.Background()
	claims := ClaimsFromContext(ctx)

	assert.Nil(t, claims, "A context without claims should not produce one")

	claims = &AuthenticationClaims{}
	ctx = context.WithValue(ctx, ctxKeyAuthentication, claims)
	assert.NotNil(t, claims, "A context with claims should produce one")
}




//
//func TestSimpleAuthenticate(t *testing.T) {
//
//	ctx := context.Background()
//
//	err := os.Setenv(envOauth2WellKnownJwkUri, "http://localhost:4444/.well-known/jwks.json")
//
//	if err != nil {
//		t.Errorf("well known JWK uri was not setable %v", err)
//		return
//	}
//
//	ctx2, err := authenticate(ctx, sampleAccessKey)
//	if err != nil {
//		t.Errorf("There was an error authenticating access key, %v", err)
//		return
//	}
//
//	claims := ClaimsFromContext(ctx2)
//	assert.NotNil(t, claims, "supplied context should contain authentication claims")
//
//}
//

