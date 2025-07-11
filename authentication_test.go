package frame_test

import (
	"testing"

	frame "github.com/pitabwire/frame"
)

const sampleAccessKey = "eyJhbGciOiJSUzI1NiIsImtpZCI6InB1YmxpYzpmODg2ZDBmNy0zYmY0LTQzMzgtOGU4Yy01ZjhjNmVlNGM3MWQiLCJ0eXAiOiJKV1QifQ.eyJhdF9oYXNoIjoicUdqdV91YnRuUkRyaGZ6WEppVzl3dyIsImF1ZCI6WyJjMmY0ajdhdTZzN2Y5MXVxbm9rZyJdLCJhdXRoX3RpbWUiOjE2MjIzMDk0OTUsImV4cCI6MTgwMjMwOTQ5OSwiaWF0IjoxNjIyMzA5NDk5LCJpc3MiOiJodHRwOi8vMTI3LjAuMC4xOjQ0NDQvIiwianRpIjoiZjM5ZGIzYTEtMmU3Ni00YzQyLWEyMmItMTg5NThiYTg3MjM1Iiwibm9uY2UiOiIiLCJyYXQiOjE2MjIzMDk0ODgsInNpZCI6ImNhNmM2NmE3LTg3MDItNDRjZS1hMTllLWRkZDJkYzQ4Y2E3MiIsInN1YiI6ImMyb2hoYzNuZGJtMGI2Y2g5dGUwIn0.BKh_m7fXaMlqXNLGisQ7vBtubgfws7h-oo9L_HXuUuY9mPs20dZ7HlQp_s-jxbdh1oDFxzRsoklbgmHglHCHBimDT3hkFPiZUmsqHtGM5P2neRBXD5ogWTjPBY_piIxu7JoB_GbFF1mZiy7Q7Lw_NpObvtLT3VC-wMMJ0fZDkyQY0hiFzLaUXVjJ96X0y0Vs0ExrcSQPnuT8CYQlhkO3qaRbKOM8p8C8IzHrmJg3N96IiZc8Vy9H9cbkmCfNlIvHx1zTIZbwyPbTjp43kI_Eo8fMmbdK_XkTnxouGtArVWoW1jjG6t4UgYafm42QJPJJvwIY2uwAg0x6B-1KwC9GgoxCGGWXRiWt9vL9ALxMpDRIxYqo2sh0OcVObvYsCTFKF8ekl5RSrvlAeu8QSkVXLvdBlaCHfvxHm2po32s6j7zvzXeuczxuiAj54Gd_7QWPwHu-2TW2gnG3oa5nbTofcmNb7Qm2QoGptIgx80gMJiCVGLCfv2UUwqZRoLzF9XkWiXKWRCq6dM4QYEIa6dyxT4BRb04W1Qcq_90Y8IsmWsXm3AQptILtDfEok93UIfnT5YnyDhAh4QmVlwCgzwokNlyd9vGtauKUZyIIKLyZ8GPCldou75GD7t4ZByUcRdHStuTvJEqJ98Fe85VolW8rubqIiN_uEzTNq5vWdFT5boo"

const sampleWellKnownJwk = "{\"keys\":[{\"use\":\"sig\",\"kty\":\"RSA\",\"kid\":\"public:f886d0f7-3bf4-4338-8e8c-5f8c6ee4c71d\",\"alg\":\"RS256\",\"n\":\"43g42b14fJGjB9wVMrYlk6L1Aig7HWt5Aere0AQQC3tdJdmzwvyCA4rYKDB2rTHSgN-xSWq12rtgrZIfjNHFj8w4p04U0aXhWFb_bVs0TTLrdlb9syAidX1H3JAwKngqHC3zkDRzVsKUKQCGSl2IScLR6B6eclgCsPL9O-SKA7BwdH3XHz3lFBhpc3Fn_TMMd2q44YQH8JWGKJPXiVHR6XB3w2IwNtFEbE3D2HpimRxP7GpGDMYFq1_eCFYUSdSK39dTfXj-SeQ0tM_voWNrS1ydH9eC5Au0zLaxWskco_8dKiGWYoQzep_Od4tlc_l2GDFoXhCOeb_6e3NENFGkef1ewPyX-hUguYm7OxBYwauBi6LTZYTkSHKocG1wT_XyE2QB7TEb2F1KQ_4WhLicGlOz4biVSs75v9FSPHFhMJ_ZpzjReYdTUJVBoQI2HMo5vElgxB79GgzCp-cp4286_OW1QnDfpvkgDIJtYnedNMmMWCpLTwtswYqO5lucCjR-jYukuV36NDuDOV1b-UqPQh95IScZUca6kVEU_5vcbiXaf24cDcVlMbN854HIYBzWinqrn_YX1mATrq6uoHT9Frth_pMJSop28iX5861p2dLdY0wBlb-x1YbQ8m5eoM7WMIDL1VVcoorecR6L_LS3App_XbaDSrtEE7wbS5iqyjk\",\"e\":\"AQAB\"},{\"use\":\"sig\",\"kty\":\"RSA\",\"kid\":\"public:0a7dad6b-ee8c-4d8a-8692-741770246f74\",\"alg\":\"RS256\",\"n\":\"x8zZj5GjuhJ4yABn2X1bCZi3jGJEIROqJxSNFt7lCi-IVMKbENudWL0HQxtnkglRitdAZXaiXToo5eWl-eWEIeK0p501PIX1Iq32BPehUI6H7t7Xth-0C65Ub2_Aho5QKCyXNH2mi75yyWIXLk0EWzgP_2H32BzS2w3OHjrogino7h0Neo98Q_727fKbTkreOLRyrvTNJWzpPrhoodz4UsT9EyY9eAW6kdaBl4k05qDm52BZM2PT4ToMeP3kMTFx_2aeoiegjaNkV2G5ONCLlYOHp3n8Hek_V_--695jaWvHgsWprykZ9KEX0nwhgy3B8DT329I9huf0vtjDqe1-Dd6qMwcMII8OT3i0_fp7rIlJUweufEIHpRVyR4KXfrsH8BP3V4Qyh_4o6IsaQpBcbNaKqSPgtOpySkJJkK0XbbRY7YcDZyCIbdpVfZHmz2sGspQ9wk0-bg8I2QJVJ6QvIP7lo_rysTjAjuMMw-e6rNuBO-zPVZumt9qOelupFhXhuk54gP_kWK3yCLdkwGHBloelbptDJLVK9IyYcPbEIxMLafIiOJ5XVFi5sEuSsvhEu9yi2M2c-BY4hlmxYuZPvGkwmxqFP2X4JQhnYAKhrdk7Hj6A2TnXhWOJu5v9L-JOkaY3MDuE3dQVMX90S_XsXD_Ew6mzUsF3BvY911glHxs\",\"e\":\"AQAB\"}]}"

const tenantWellKnownJwk = "{\"keys\":[{\"use\":\"sig\",\"kty\":\"RSA\",\"kid\":\"30c43677-4c0d-4191-bff1-5ced49059f4f\",\"alg\":\"RS256\",\"n\":\"5E0LW8-pnWJJof9SWVmyOfnijHXeJn7ZUZ0FywAzKjgj5oGJvxMQO0mxa3OTySTkHl0keJecEfxhbHyQOmt_RsGXfjOaKTsVblJwgEyC_LxFn4qCP1KV5m1G_2uSNoImMrBrWXYDwt7cd3Bvk7cUHUrW5YINqpNGv1-BPobq2gCI06mJxESyWH0qJYrCWhJoXUp_pZp4UFa1IzLqK_V0m6kIVg-ad_F6Lzd0MDq2DSRM-iNpQLURGAlvTZKyJEchVG17t4zK9bq21WgF7ses8_zDY_-2xyPPUNRMR76dAHUdnJrBd08XhDuq8ddL_Lg3ZLX7HJBUgpiZuC1W1mf8o39AimnCffXJs73ZtNf3kua9O1SyeE1q3nmB0aXFNvrT_VeTP22bOwI3dIf39esiCTI3HyT_GzbzkXJOAbsCrDgK6tRZlXGJRU80drlfdbI3ZOUHBQiwxwOZ0Pp7NSPD-hPo6YIUoRqcz1heu-uvXQ3cvm0dZhbeuvjYAdxwUL9nofRE65v_8A-wCB1WaG6CBBYHcOr6Prt5ts-W7WKy0nlmRnGCyMzykwj4PTGH0H0PzOxg2IMqQ_gc8ybwT3xgotSUNDCMgMpKfY-1-Vre1cKTFH6Hthx_AWrJeFdEzdvHes75uAX9HC2TQe6BFmUC-GANPif5MitNzDfU_l7-LG8\",\"e\":\"AQAB\"},{\"use\":\"sig\",\"kty\":\"RSA\",\"kid\":\"b45200ad-56d4-4c4e-bc1a-de2181deddbe\",\"alg\":\"RS256\",\"n\":\"uElbd6H4X8uOUBEIS-QrLr-_WfcuvCKJdDCdLGATtiOhtsXEgyYdcaYZbbPOK5Jgf8xEW2qwbvubW7BrVG71zYC9KN_re2pp6_Dcj9qf3h_jOTOv4hLN331uD8c3MXIoHqOV9g7tZaWLL4Cw0tIaSU4h4pMXnYHs0LE9xbi7561DdLXIWPrg7LElbvTkAxpP6aJ8C8ehKDeAIp4QkLE2JXRSHIlwSp4cglVlmujy1ypk4r06YH93aZx9vSXQiNj6sEArllWSe_eqx_B4_dqOTEXJmkVTkzOmOFNpClJ4q8Ih4u1XFV7NQPS-OqIFSKLCuQ-5_yH-Xh1Ny9JhvML4XORJJrs1QC46j7akFr8oH4ttoPpSVmavTP_D2funbEaU6r7k8PTSaa-XfbBS_6PI2Aqj7qzKSBc93tOpg7HIuBo7PRggO-RuGgBK15xpw1VtbxOx8DXnsZnTtyAIxazoilMIsf4QoNF7F-cFH9RsSWy0xq8NbEnfjSjCsofdURsVm4XLpIySfwFC2i08nPX8E6VIOik4MojOkfpWwoJa8eZw3oetTE6sExB17UKYQTgfIhl7q2C1wLaDN-NwwJZRT6d8Hwesy1jasqzQ9GxM0aRPEp4rrcMnUQfc2U05-YWc91-dFk5SrjLmJ5JfJc0q8w_jQdvB6HOGsyhQyNa9gN8\",\"e\":\"AQAB\"}]}"

func TestAuthenticationFromContext(t *testing.T) {
	ctx := t.Context()
	claims := frame.ClaimsFromContext(ctx)

	if claims != nil {
		t.Errorf("A context without claims should not produce one")
	}

	claims = &frame.AuthenticationClaims{}
	ctx = claims.ClaimsToContext(ctx)

	if frame.ClaimsFromContext(ctx) == nil {
		t.Errorf("A context with claims should produce one")
	}
}

func TestSimpleAuthenticate(t *testing.T) {
	ctx, srv := frame.NewService("Test Srv", frame.WithConfig(
		&frame.ConfigurationDefault{Oauth2WellKnownJwkData: sampleWellKnownJwk}))

	ctx2, err := srv.Authenticate(ctx, sampleAccessKey, "", "")
	if err != nil {
		t.Errorf("There was an error authenticating access key, %s", err)
		return
	}

	claims := frame.ClaimsFromContext(ctx2)
	if claims == nil {
		t.Errorf("supplied context should contain authentication claims")
	}
}

func TestSimpleAuthenticateWithAudience(t *testing.T) {
	ctx, srv := frame.NewService("Test Srv", frame.WithConfig(
		&frame.ConfigurationDefault{Oauth2WellKnownJwkData: sampleWellKnownJwk}))

	ctx2, err := srv.Authenticate(ctx, sampleAccessKey, "c2f4j7au6s7f91uqnokg", "")
	if err != nil {
		t.Errorf("There was an error authenticating access key due to audience, %s", err)
		return
	}

	claims := frame.ClaimsFromContext(ctx2)
	if claims == nil {
		t.Errorf("supplied context should contain authentication claims")
	}
}

func TestSimpleAuthenticateWithIssuer(t *testing.T) {
	ctx, srv := frame.NewService("Test Srv", frame.WithConfig(
		&frame.ConfigurationDefault{Oauth2WellKnownJwkData: sampleWellKnownJwk}))

	ctx2, err := srv.Authenticate(ctx, sampleAccessKey, "", "http://127.0.0.1:4444/")
	if err != nil {
		t.Errorf("There was an error authenticating access key due to issuer, %s", err)
		return
	}

	claims := frame.ClaimsFromContext(ctx2)

	if claims == nil {
		t.Errorf("supplied context should contain authentication claims")
	}
}

func TestSimpleAuthenticateWithOIDC(t *testing.T) {
	t.Setenv("OAUTH2_SERVICE_URI", "https://accounts.google.com")

	ctx := t.Context()
	cfg, err := frame.ConfigLoadWithOIDC[frame.ConfigurationDefault](ctx)
	if err != nil {
		t.Error(err)
	}

	t.Logf("Configuration  : %s", cfg.GetOauth2UserInfoEndpoint())
	t.Logf("Configuration JWK: %s", cfg.GetOauth2WellKnownJwkData())
}

func TestAuthenticateWithTenantClaims(t *testing.T) {
	t.Skip("To run this test manually comment out this line")

	tenantAccessKey := "eyJhbGciOiJSUzI1NiIsImtpZCI6ImI0NTIwMGFkLTU2ZDQtNGM0ZS1iYzFhLWRlMjE4MWRlZGRiZSIsInR5cCI6IkpXVCJ9.eyJhdWQiOlsic2VydmljZV9jaGF0X2VuZ2luZSIsInNlcnZpY2VfcHJvZmlsZSIsInNlcnZpY2Vfc3Rhd2lfYXBpIiwic2VydmljZV9maWxlcyJdLCJjbGllbnRfaWQiOiI0NzUxZWEzZC1lNGU0LTQyZmUtOWIwMC0xMGFlYTVmZTk5ZmEiLCJleHAiOjE3MDU4OTc1MTAsImV4dCI6eyJhY2Nlc3NfaWQiOiJjbHQwcGUxbzc0dHM3M2J0cnVtMCIsImFjY2Vzc19zdGF0ZSI6IkNSRUFURUQiLCJwYXJ0aXRpb25faWQiOiI5YnN2MHMwaGlqamcwMnFrczZpMCIsInBhcnRpdGlvbl9zdGF0ZSI6IkNSRUFURUQiLCJyb2xlIjoidXNlciIsInRlbmFudF9pZCI6Ijlic3YwczBoaWpqZzAycWtzNmRnIn0sImlhdCI6MTcwNTg5MzkxMCwiaXNzIjoiaHR0cHM6Ly9vYXV0aDIuc3Rhd2kuaW8vIiwianRpIjoiZWVhNmZkNGItMWFiMS00NDFmLThlY2QtMWIxYjQxYmMwZWI4Iiwic2NwIjpbIm9wZW5pZCIsInByb2ZpbGUiLCJjb250YWN0Iiwib2ZmbGluZV9hY2Nlc3MiXSwic3ViIjoiY2x0MHA5dmlvcGZjNzNmZG9hZzAifQ.VuUw35N9HghZecYqR-L4bQJqZxAoyDj8b9e01bTGgP9ppM7kT5FHNHfaXP3vLQg8lRym8u_AA7XkL2IyG0EKcqmNCJCLeVpzp9aOx1TLm8Zu-b4aRBnkjEQ8gNEfOfl_7c1voK_e2EKkO6E_CE2qfxwESs-b6FmcxY-AvgX8S-IT9eYjSEPLKpEV0l8JhzdJj3i7YdetCQVtmm3uum3jAIMoWUkszURERGyG80ZSr2NE0H1V_OUPWTwE1ysML_YpwDrCtEb6BT2B-3cRnRFjJkyR0D5Dr62GEUOV3w82InbfrQwa09m3zViYX4AYMmE6Oj6ZqHoU2GVRhTdaQXxzeZOyecKDojfeEov_qro9nzJR8olJE_VlHYIpho2AlKu1DFqy7-OTHoO_9N3KvEgTcVW8wi7-ojbI_sJILkn4EVH1Ua-uOPNhAPgpSFrkgehsse4AEhK5n7lGHrj1D1QGhc97gxsSuG5Ybjd-DKascnZMcXZ6G0wXw16JCy6rHOn5iDhD6Nh4GYKF7MUDMgj-S-sgldl5FSM2tHXBAA0mxEXG5f8kz90j63eUTtYobDRj6zYfRsdidoD8R3sW5ELhbP1tFuxZGZGt-La_QF_73Xa62UoI5rliKvsDRMabAsEmnn1cGj-UAersuw9loT75GKlrB8311Ye_ODejVzVQnGc"

	ctx, srv := frame.NewService("Tenant Srv", frame.WithConfig(
		&frame.ConfigurationDefault{Oauth2WellKnownJwkData: tenantWellKnownJwk}))

	ctx2, err := srv.Authenticate(ctx, tenantAccessKey, "", "https://oauth2.stawi.io/")
	if err != nil {
		t.Errorf("There was an error authenticating access key due to issuer, %s", err)
		return
	}

	claims := frame.ClaimsFromContext(ctx2)

	if claims == nil {
		t.Errorf("supplied context should contain authentication claims")
	}

	if claims.GetTenantID() == "" {
		t.Errorf("auth claim has no tenant Id")
	}

	if claims.GetPartitionID() == "" {
		t.Errorf("auth claim has no partition Id")
	}

	if claims.GetAccessID() == "" {
		t.Errorf("auth claim has no access Id")
	}

	if len(claims.GetRoles()) == 0 {
		t.Errorf("auth claim has no roles")
	}
}
