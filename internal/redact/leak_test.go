package redact_test

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/pitabwire/frame/v3/internal/redact"
)

// redactedOutputs returns every surface a credential-bearing raw URL/DSN
// can reach logs through: URL(), Text() over plain and %q forms, and
// Error() over a %q-wrapped error and the real url.Parse error.
func redactedOutputs(raw string) map[string]string {
	outs := map[string]string{
		"URL":       redact.URL(raw),
		"Text":      redact.Text(raw, "dial "+raw+" failed"),
		"TextQuote": redact.Text(raw, fmt.Sprintf("parse %q: bad", raw)),
		"ErrQuote":  redact.Error(raw, fmt.Errorf("parse %q: bad", raw)).Error(),
	}
	if _, err := url.Parse(raw); err != nil {
		outs["ParseErr"] = redact.Error(raw, err).Error()
	}
	return outs
}

func quotedInner(s string) string {
	q := strconv.Quote(s)
	return q[1 : len(q)-1]
}

func assertNoLeak(t *testing.T, raw, secret string) {
	t.Helper()
	for surface, out := range redactedOutputs(raw) {
		if strings.Contains(out, secret) || strings.Contains(out, quotedInner(secret)) {
			t.Errorf("%s leaks %q: raw=%q out=%q", surface, secret, raw, out)
		}
	}
}

func TestRedaction_NoLeakAcrossSurfaces(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw, secret string
	}{
		// bare tokens mis-split by reserved characters
		{"nats://tok/en123@host:4222", "tok/en123"},
		{"redis://tok?en123@host:6379", "tok?en123"},
		{"redis://tok#en123@host:6379", "tok#en123"},
		{"nats://abc/def@host", "abc/def"},
		// passwords with reserved characters
		{"postgres://user:p/ss@host/db", "p/ss"},
		{"postgres://user:p#ss@host/db", "p#ss"},
		{"postgres://user:12#ss@host/db", "12#ss"},
		{"postgres://user:p?ss@host/db", "p?ss"},
		{`postgres://user:p"ss@host/db`, `p"ss`},
		{"postgres://user:p@ss@host/db", "p@ss"},
		{"postgres://user:p\x01ss@host/db", "p\x01ss"},
		{"postgres://user:p\nss@host/db", "p\nss"},
		{"postgres://user:p@ss/w0rd@host:5432/db", "w0rd"},
		{"postgres://user:p@ss#w0rd@host:5432/db", "w0rd"},
		{"postgres://user:p@ss?w0rd@host:5432/db", "w0rd"},
		{"postgres://user:ab#cd@host/db", "ab#cd"},
		{"postgres://user:p%2Fss@host/db", "p/ss"},
		{"postgres://user:pa%zzss@host/db", "pa%zzss"},
		{"amqp://user:pass word1@host/", "pass word1"},
		{"postgres://:onlypass9@host/db", "onlypass9"},
		{"postgres://u:pw@h1,u2:pw9@h2/db", "pw9"},
		// username containing '#'
		{"a://#:0000@host", "0000"},
		{"a://0:@#00@host:5432/db", "@#00"},
		// query strings, fragments and keyword DSNs
		{"postgres://user:secr3t@host/db?password=other1", "other1"},
		{"postgres://user@host/db?password=q123", "q123"},
		{"postgres://u:pw1@h/db?sslkey=/x&sslpassword=kp99", "kp99"},
		{"https://h/x?X-Amz-Signature=abcd9", "abcd9"},
		{"https://h/x#access_token=frag99", "frag99"},
		{"https://h/cb#id_token=idfrag7&state=s", "idfrag7"},
		{"host=x password=sekrit1 user=u", "sekrit1"},
		{"host=x sslpassword=sekrit2 user=u", "sekrit2"},
		{"host=x password='se cr3t' user=u", "se cr3t"},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()
			assertNoLeak(t, tc.raw, tc.secret)
		})
	}
}

// FuzzURLUserinfo checks the core property: no password (or bare-token
// userinfo) survives on any redaction surface, whatever characters it holds.
// CI runs the seed corpus with every go test; `make fuzz-redact` runs it.
func FuzzURLUserinfo(f *testing.F) {
	seeds := [][3]string{
		{"user", "p/ss", "postgres"},
		{"", "tok#en", "nats"},
		{"svc", "p@ss/w0rd", "postgres"},
		{"#", "0000", "a"},
		{"0", "@#00", "a"},
		{"u", "pa%zzss", "postgres"},
		{"u", "p\"s\ns", "redis"},
		{"", "tok?en123", "redis"},
	}
	for _, s := range seeds {
		f.Add(s[0], s[1], s[2])
	}

	f.Fuzz(func(t *testing.T, user, pass, scheme string) {
		if len(pass) < 4 || scheme == "" || strings.ContainsAny(user, ":@") {
			return
		}
		for _, r := range scheme {
			if r < 'a' || r > 'z' {
				return
			}
		}

		const tail = "@host:5432/db"
		raw := scheme + "://" + user + ":" + pass + tail
		// Everything an output may legitimately contain besides the secret.
		safe := strings.Join([]string{
			"dial", "failed", "parse", "bad", scheme + "://" + user + ":" + redact.Placeholder + tail,
			scheme + "://" + redact.Placeholder + tail,
			url.User(user).String(), // net/url re-escapes the username it keeps
			strconv.Quote(scheme + "://" + redact.Placeholder + tail),
		}, "\x00")
		if strings.Contains(safe, pass) || strings.Contains(safe, quotedInner(pass)) ||
			strings.Contains(pass, redact.Placeholder) || strings.Trim(pass, "x") == "" {
			return
		}

		for surface, out := range redactedOutputs(raw) {
			if strings.Contains(out, pass) || strings.Contains(out, quotedInner(pass)) {
				t.Fatalf("%s leaks password: raw=%q out=%q", surface, raw, out)
			}
		}
	})
}
