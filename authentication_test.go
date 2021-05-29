package frame

import (
	"context"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

const sampleAccessKey = "eyJhbGciOiJSUzI1NiIsImtpZCI6InB1YmxpYzpmODg2ZDBmNy0zYmY0LTQzMzgtOGU4Yy01ZjhjNmVlNGM3MWQiLCJ0eXAiOiJKV1QifQ.eyJhdF9oYXNoIjoiVndBTlk1TmVRdXFGWFVNYW5Nb19xdyIsImF1ZCI6WyJjMmY0ajdhdTZzN2Y5MXVxbm9rZyJdLCJhdXRoX3RpbWUiOjE2MjIyNzYyOTQsImV4cCI6MTYyMjI3OTg5OCwiaWF0IjoxNjIyMjc2Mjk4LCJpc3MiOiJodHRwOi8vMTI3LjAuMC4xOjQ0NDQvIiwianRpIjoiYzk0NjQ2MGQtMTc1OC00OTdiLThjODItODRlZjhhYWMyMTcyIiwibm9uY2UiOiIiLCJyYXQiOjE2MjIyNzYyODcsInNpZCI6Ijk0MDJiNWE0LWNkNmMtNDgxOC04Yzk1LWJmNGNjOTY5ZjFhZSIsInN1YiI6ImMyb2hoYzNuZGJtMGI2Y2g5dGUwIn0.qsXICYkU4r1FAx3u_XcQmg3l36dIYODoqczWLnVqyD-2q7YtYL2SKRyKL9IynD3jm7MEH7-YH2J2l7Dyir6-EegSEjoIf5DGRYX1DhCWmaYrr0l4061KXLm424cphS7yacos4lxHAh54v91fmvJOwA6q1kdN9hYzEL7nfanDEzAfhuLGfuGFcmcGS7-LNONRkE-p4B8gx5PU3Ele_qZMaQcLyYL4oki548Gwep9WsQ_dOowpAE_CoSTeGhP-Ppon2JtZzETiZ0QKwxtXVX6YaqgYxaS4CX-YK8i337ZZ3fusmo1IhOTBI4vpSYPPkPFFPtf4ortzMLAch7ZBTBE767-RfGIYIOss3fZ7cbv7GJEOKR2Q2zmBpCyCEU9I3n33e_G4ST9PwRypf1W-aXn6CV10BUtaIJW1WNO9B_qEUPpl49hbISEi3JA_Zf2tW3yUT8rcS1b3St0RvfJakSH8246CSxIKHxK0hKpEtlpxXs7fPmbrL9LRi0jkMmYx3iAW7rbKyBYn993KlNtgm8hJx2jVAnIUwx61Xnl4M0Et79byft5RcyLevSGC9ODVy0qhE2abTyOQTsm4ucaFG3bh3SIxQ3wR-g-P_WVAsLdO08G0m44uObULBbxfDfboG78bXf7yZVQYGfa-ArQlG5STy05bGcVN2S15Yxq2yCUa8xw"
const sampleWellKnownJwk = "{\"keys\":[{\"use\":\"sig\",\"kty\":\"RSA\",\"kid\":\"public:f886d0f7-3bf4-4338-8e8c-5f8c6ee4c71d\",\"alg\":\"RS256\",\"n\":\"43g42b14fJGjB9wVMrYlk6L1Aig7HWt5Aere0AQQC3tdJdmzwvyCA4rYKDB2rTHSgN-xSWq12rtgrZIfjNHFj8w4p04U0aXhWFb_bVs0TTLrdlb9syAidX1H3JAwKngqHC3zkDRzVsKUKQCGSl2IScLR6B6eclgCsPL9O-SKA7BwdH3XHz3lFBhpc3Fn_TMMd2q44YQH8JWGKJPXiVHR6XB3w2IwNtFEbE3D2HpimRxP7GpGDMYFq1_eCFYUSdSK39dTfXj-SeQ0tM_voWNrS1ydH9eC5Au0zLaxWskco_8dKiGWYoQzep_Od4tlc_l2GDFoXhCOeb_6e3NENFGkef1ewPyX-hUguYm7OxBYwauBi6LTZYTkSHKocG1wT_XyE2QB7TEb2F1KQ_4WhLicGlOz4biVSs75v9FSPHFhMJ_ZpzjReYdTUJVBoQI2HMo5vElgxB79GgzCp-cp4286_OW1QnDfpvkgDIJtYnedNMmMWCpLTwtswYqO5lucCjR-jYukuV36NDuDOV1b-UqPQh95IScZUca6kVEU_5vcbiXaf24cDcVlMbN854HIYBzWinqrn_YX1mATrq6uoHT9Frth_pMJSop28iX5861p2dLdY0wBlb-x1YbQ8m5eoM7WMIDL1VVcoorecR6L_LS3App_XbaDSrtEE7wbS5iqyjk\",\"e\":\"AQAB\"},{\"use\":\"sig\",\"kty\":\"RSA\",\"kid\":\"public:0a7dad6b-ee8c-4d8a-8692-741770246f74\",\"alg\":\"RS256\",\"n\":\"x8zZj5GjuhJ4yABn2X1bCZi3jGJEIROqJxSNFt7lCi-IVMKbENudWL0HQxtnkglRitdAZXaiXToo5eWl-eWEIeK0p501PIX1Iq32BPehUI6H7t7Xth-0C65Ub2_Aho5QKCyXNH2mi75yyWIXLk0EWzgP_2H32BzS2w3OHjrogino7h0Neo98Q_727fKbTkreOLRyrvTNJWzpPrhoodz4UsT9EyY9eAW6kdaBl4k05qDm52BZM2PT4ToMeP3kMTFx_2aeoiegjaNkV2G5ONCLlYOHp3n8Hek_V_--695jaWvHgsWprykZ9KEX0nwhgy3B8DT329I9huf0vtjDqe1-Dd6qMwcMII8OT3i0_fp7rIlJUweufEIHpRVyR4KXfrsH8BP3V4Qyh_4o6IsaQpBcbNaKqSPgtOpySkJJkK0XbbRY7YcDZyCIbdpVfZHmz2sGspQ9wk0-bg8I2QJVJ6QvIP7lo_rysTjAjuMMw-e6rNuBO-zPVZumt9qOelupFhXhuk54gP_kWK3yCLdkwGHBloelbptDJLVK9IyYcPbEIxMLafIiOJ5XVFi5sEuSsvhEu9yi2M2c-BY4hlmxYuZPvGkwmxqFP2X4JQhnYAKhrdk7Hj6A2TnXhWOJu5v9L-JOkaY3MDuE3dQVMX90S_XsXD_Ew6mzUsF3BvY911glHxs\",\"e\":\"AQAB\"}]}"

func TestAuthenticationFromContext(t *testing.T) {
	ctx := context.Background()
	claims := ClaimsFromContext(ctx)

	assert.Nil(t, claims, "A context without claims should not produce one")

	claims = &AuthenticationClaims{}
	ctx = context.WithValue(ctx, ctxKeyAuthentication, claims)
	assert.NotNil(t, claims, "A context with claims should produce one")
}





func TestSimpleAuthenticate(t *testing.T) {

	ctx := context.Background()

	err := os.Setenv(envOauth2WellKnownJwk, sampleWellKnownJwk)

	if err != nil {
		t.Errorf("well known JWK uri was not setable %v", err)
		return
	}

	ctx2, err := authenticate(ctx, sampleAccessKey)
	if err != nil {
		t.Errorf("There was an error authenticating access key, %v", err)
		return
	}

	claims := ClaimsFromContext(ctx2)
	assert.NotNil(t, claims, "supplied context should contain authentication claims")

}


