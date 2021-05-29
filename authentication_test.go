package frame

import (
	"context"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

const sampleAccessKey = "eyJhbGciOiJSUzI1NiIsImtpZCI6InB1YmxpYzpmODg2ZDBmNy0zYmY0LTQzMzgtOGU4Yy01ZjhjNmVlNGM3MWQiLCJ0eXAiOiJKV1QifQ.eyJhdF9oYXNoIjoiMTQ3NDlSeGx6bXZkeWZyNjFoZXdDdyIsImF1ZCI6WyJjMmY0ajdhdTZzN2Y5MXVxbm9rZyJdLCJhdXRoX3RpbWUiOjE2MjIyNzY3NzUsImV4cCI6MTYyMjI4MDM3OSwiaWF0IjoxNjIyMjc2Nzc5LCJpc3MiOiJodHRwOi8vMTI3LjAuMC4xOjQ0NDQvIiwianRpIjoiNTlhZTk1YTgtNDI4OC00YjIzLThjZGUtYzk5ZjA2MDIxNzExIiwibm9uY2UiOiIiLCJyYXQiOjE2MjIyNzY3NjgsInNpZCI6IjExYWYzYTA4LWMxYTItNGY2Mi04NGM0LWNiYThjZTY5MmM4ZCIsInN1YiI6ImMyb2hoYzNuZGJtMGI2Y2g5dGUwIn0.PV4mufJ1mVt961Uv4EY4s3gSiidMtBH2HCIHgGMgn8LLD4unOF7uSc1YPo9rsi603eRf6UE9LYKZzfH9z8Pc-mDblk-4QjIF7dsPMquOdMlShTvOvmXc-1j2msIu3mI-2boxLhs8oYB6M5yQZ_rS6pZqPeHfpU3vtJP_sT1iEVOmy7h3oisItEZpT7t5H_WxVXGxpZnXJJk1aaMLjbLY7KU0vSg3109_dzsro_61bHTV7Ja-py2jxQp3bzaKk0PMoAYWkkRhmNoLdLNMizIko9rOO4mKxBQsLreDaG0oaEiqxTL0DwRvsGKSMHQK0mL2ucYg2fbYerz81biGdweAmgiNdIOzQg32YO32_ZQlGVGU-znsBassOd1UNxx6n8ws_x3d5gKRjux0xTAFNhYqNBAY_CT3_gSBDjkBORnQFCBMqyn3luRZj23kHgxiig--q9gaMszAw1fmW2D2feO1qVWS10kGPJSVYmLyiVUJXWdujnk2vgeXZ5YTbTSkRJHQhbuimodgXva7I5jsGCDjVT4D5ttZk7M4qDblE37NO88a6w1p9qyZqYEnucLZqgYqh0jRTfxkfswuL89Xqb8qa-IrMIu35-iB5NELgH9EB2ru7sRyAxs8Peao_lOpfeo-mp46KPtWZe0veRs_Tuiz6hH4f1CHZiOYnUyYHZFwueQ"
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


