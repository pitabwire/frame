package frame

import (
	"context"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

const sampleAccessKey = "eyJhbGciOiJSUzI1NiIsImtpZCI6InB1YmxpYzpmODg2ZDBmNy0zYmY0LTQzMzgtOGU4Yy01ZjhjNmVlNGM3MWQiLCJ0eXAiOiJKV1QifQ.eyJhdF9oYXNoIjoicUdqdV91YnRuUkRyaGZ6WEppVzl3dyIsImF1ZCI6WyJjMmY0ajdhdTZzN2Y5MXVxbm9rZyJdLCJhdXRoX3RpbWUiOjE2MjIzMDk0OTUsImV4cCI6MTgwMjMwOTQ5OSwiaWF0IjoxNjIyMzA5NDk5LCJpc3MiOiJodHRwOi8vMTI3LjAuMC4xOjQ0NDQvIiwianRpIjoiZjM5ZGIzYTEtMmU3Ni00YzQyLWEyMmItMTg5NThiYTg3MjM1Iiwibm9uY2UiOiIiLCJyYXQiOjE2MjIzMDk0ODgsInNpZCI6ImNhNmM2NmE3LTg3MDItNDRjZS1hMTllLWRkZDJkYzQ4Y2E3MiIsInN1YiI6ImMyb2hoYzNuZGJtMGI2Y2g5dGUwIn0.BKh_m7fXaMlqXNLGisQ7vBtubgfws7h-oo9L_HXuUuY9mPs20dZ7HlQp_s-jxbdh1oDFxzRsoklbgmHglHCHBimDT3hkFPiZUmsqHtGM5P2neRBXD5ogWTjPBY_piIxu7JoB_GbFF1mZiy7Q7Lw_NpObvtLT3VC-wMMJ0fZDkyQY0hiFzLaUXVjJ96X0y0Vs0ExrcSQPnuT8CYQlhkO3qaRbKOM8p8C8IzHrmJg3N96IiZc8Vy9H9cbkmCfNlIvHx1zTIZbwyPbTjp43kI_Eo8fMmbdK_XkTnxouGtArVWoW1jjG6t4UgYafm42QJPJJvwIY2uwAg0x6B-1KwC9GgoxCGGWXRiWt9vL9ALxMpDRIxYqo2sh0OcVObvYsCTFKF8ekl5RSrvlAeu8QSkVXLvdBlaCHfvxHm2po32s6j7zvzXeuczxuiAj54Gd_7QWPwHu-2TW2gnG3oa5nbTofcmNb7Qm2QoGptIgx80gMJiCVGLCfv2UUwqZRoLzF9XkWiXKWRCq6dM4QYEIa6dyxT4BRb04W1Qcq_90Y8IsmWsXm3AQptILtDfEok93UIfnT5YnyDhAh4QmVlwCgzwokNlyd9vGtauKUZyIIKLyZ8GPCldou75GD7t4ZByUcRdHStuTvJEqJ98Fe85VolW8rubqIiN_uEzTNq5vWdFT5boo"
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
		t.Errorf("well known JWK uri was not setable %+v", err)
		return
	}

	ctx2, err := authenticate(ctx, sampleAccessKey, "", "")
	if err != nil {
		t.Errorf("There was an error authenticating access key, %+v", err)
		return
	}

	claims := ClaimsFromContext(ctx2)
	assert.NotNil(t, claims, "supplied context should contain authentication claims")

}

func TestSimpleAuthenticateWithAudience(t *testing.T) {

	ctx := context.Background()

	err := os.Setenv(envOauth2WellKnownJwk, sampleWellKnownJwk)

	if err != nil {
		t.Errorf("well known JWK uri was not setable %+v", err)
		return
	}

	ctx2, err := authenticate(ctx, sampleAccessKey, "c2f4j7au6s7f91uqnokg", "")
	if err != nil {
		t.Errorf("There was an error authenticating access key due to audience, %+v", err)
		return
	}

	claims := ClaimsFromContext(ctx2)
	assert.NotNil(t, claims, "supplied context should contain authentication claims")

}

func TestSimpleAuthenticateWithIssuer(t *testing.T) {

	ctx := context.Background()

	err := os.Setenv(envOauth2WellKnownJwk, sampleWellKnownJwk)

	if err != nil {
		t.Errorf("well known JWK uri was not setable %+v", err)
		return
	}

	ctx2, err := authenticate(ctx, sampleAccessKey, "", "http://127.0.0.1:4444/")
	if err != nil {
		t.Errorf("There was an error authenticating access key due to issuer, %+v", err)
		return
	}

	claims := ClaimsFromContext(ctx2)
	assert.NotNil(t, claims, "supplied context should contain authentication claims")

}
