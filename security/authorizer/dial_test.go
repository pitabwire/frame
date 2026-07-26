package authorizer //nolint:testpackage // white-box tests for unexported dial helpers

import (
	"testing"
)

func TestKetoDialOptionsHTTPUsesInsecure(t *testing.T) {
	target, opts := ketoDialOptions("http://keto-write:4467")
	if target != "keto-write:4467" {
		t.Fatalf("target = %q, want keto-write:4467", target)
	}
	if len(opts) != 1 {
		t.Fatalf("opts len = %d, want 1 (insecure only)", len(opts))
	}
}

func TestKetoDialOptionsHTTPSUsesTLS(t *testing.T) {
	target, opts := ketoDialOptions("https://authz-w.stawi.org")
	if target != "authz-w.stawi.org" {
		t.Fatalf("target = %q, want authz-w.stawi.org", target)
	}
	// TLS always present; ID token may be absent outside GCP (opts len 1 or 2).
	if len(opts) < 1 {
		t.Fatalf("opts empty")
	}
}

func TestKetoDialOptionsBareHostInsecure(t *testing.T) {
	target, opts := ketoDialOptions("127.0.0.1:4466")
	if target != "127.0.0.1:4466" {
		t.Fatalf("target = %q", target)
	}
	if len(opts) != 1 {
		t.Fatalf("opts len = %d, want 1", len(opts))
	}
}

func TestGrpcTargetStripsScheme(t *testing.T) {
	cases := map[string]string{
		"https://authz.stawi.org": "authz.stawi.org",
		"http://keto-read:4466":   "keto-read:4466",
		"https://x.run.app:443":   "x.run.app:443",
		"plain-host:99":           "plain-host:99",
	}
	for in, want := range cases {
		if got := grpcTarget(in); got != want {
			t.Errorf("grpcTarget(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIdTokenRPCCredentialsRequireTLS(t *testing.T) {
	c := &idTokenRPCCredentials{}
	if !c.RequireTransportSecurity() {
		t.Fatal("ID token credentials must require transport security")
	}
}
