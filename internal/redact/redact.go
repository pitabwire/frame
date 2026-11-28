// Package redact removes credentials from URLs, DSNs, error messages and
// headers before they are logged or returned to callers.
package redact

import (
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Placeholder replaces credential material in redacted output.
const Placeholder = "xxxxx"

//nolint:gochecknoglobals // immutable lookup table
var sensitiveQueryParams = map[string]struct{}{
	"password":             {},
	"pass":                 {},
	"passwd":               {},
	"pwd":                  {},
	"secret":               {},
	"client_secret":        {},
	"token":                {},
	"access_token":         {},
	"refresh_token":        {},
	"auth_token":           {},
	"api_key":              {},
	"apikey":               {},
	"key":                  {},
	"sig":                  {},
	"signature":            {},
	"x-amz-signature":      {},
	"x-amz-credential":     {},
	"x-amz-security-token": {},
	"x-goog-signature":     {},
	"x-goog-credential":    {},
	"awsaccesskeyid":       {},
	"credentials":          {},
	"nkey":                 {},
	"jwt":                  {},
}

// keywordPasswordPattern matches password=value in libpq keyword/value DSNs.
var keywordPasswordPattern = regexp.MustCompile(`(?i)(\bpassword\s*=\s*)('(?:[^'\\]|\\.)*'|\S+)`)

// URL returns raw with credentials removed so it is safe to log or embed
// in errors: the userinfo password and sensitive query parameters are
// replaced with Placeholder, as is the password of libpq keyword/value DSNs.
// A username without a password is not treated as a credential.
//
// Passwords containing reserved characters ('/', '?', '#', '"', '@', ...)
// make net/url reject the string or mis-split it into host and path. In
// those cases everything between "://" and the last '@' is masked, which may
// over-redact but never leaks.
func URL(raw string) string {
	if raw == "" {
		return raw
	}

	if !strings.Contains(raw, "://") {
		return keywordPasswordPattern.ReplaceAllString(raw, "${1}"+Placeholder)
	}

	u, err := url.Parse(raw)
	if err != nil || misparsedUserinfo(u) {
		return maskUpToLastAt(raw)
	}

	if u.User != nil {
		if _, hasPassword := u.User.Password(); hasPassword {
			u.User = url.UserPassword(u.User.Username(), Placeholder)
		}
	}

	if u.RawQuery != "" {
		u.RawQuery = redactQuery(u.RawQuery)
	}

	return u.String()
}

// misparsedUserinfo detects a URL that net/url accepted but split inside a
// password: no userinfo, a host:port-shaped authority, and an '@' that
// ended up in the path, query or fragment (e.g. user:12/ss@host).
func misparsedUserinfo(u *url.URL) bool {
	if u.User != nil || !strings.Contains(u.Host, ":") {
		return false
	}
	return strings.Contains(u.Path, "@") || strings.Contains(u.RawQuery, "@") ||
		strings.Contains(u.Fragment, "@")
}

// maskUpToLastAt masks everything between "://" and the last '@', and the
// sensitive query parameters after it.
func maskUpToLastAt(raw string) string {
	scheme, rest, _ := strings.Cut(raw, "://")
	tail := rest
	prefix := ""
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		prefix = Placeholder
		tail = rest[at:]
	}
	if q := strings.IndexByte(tail, '?'); q >= 0 {
		tail = tail[:q+1] + redactQuery(tail[q+1:])
	}
	return scheme + "://" + prefix + tail
}

// Error returns err with every credential of the raw URL/DSN removed from its
// message. Drivers and net/url routinely echo the full connection URL in
// errors, so errors derived from a DSN must be scrubbed before they are
// logged or surfaced. The original error stays reachable through Unwrap for
// errors.Is / errors.As. Errors that contain no credential are returned as-is.
//
// Limitation: errors.As (or errors.Unwrap) hands back the original,
// unscrubbed error, so calling Error() on the unwrapped value (for example a
// *url.Error) still yields the credential. Log the scrubbed error itself.
func Error(raw string, err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	scrubbed := Text(raw, msg)
	if scrubbed == msg {
		return err
	}
	return &scrubbedError{msg: scrubbed, err: err}
}

// Text removes every credential of the raw URL/DSN from text, including the
// %q-quoted forms that net/url and fmt produce (url.Error quotes the URL, so
// a '"' or control character in a password is escaped there).
func Text(raw, text string) string {
	if raw == "" || text == "" {
		return text
	}

	redacted := URL(raw)
	text = strings.ReplaceAll(text, strconv.Quote(raw), strconv.Quote(redacted))
	text = strings.ReplaceAll(text, quotedInner(raw), quotedInner(redacted))
	text = strings.ReplaceAll(text, raw, redacted)
	for _, secret := range secrets(raw) {
		text = strings.ReplaceAll(text, quotedInner(secret), Placeholder)
		text = strings.ReplaceAll(text, secret, Placeholder)
	}
	return text
}

// quotedInner returns s as it appears inside a Go-quoted string.
func quotedInner(s string) string {
	q := strconv.Quote(s)
	return q[1 : len(q)-1]
}

// secrets returns the credential values embedded in raw, in both raw and
// percent-decoded forms, longest first so overlapping values are fully masked.
func secrets(raw string) []string {
	var found []string
	found = append(found, userinfoSecrets(raw)...)
	found = append(found, querySecrets(raw)...)
	for _, m := range keywordPasswordPattern.FindAllStringSubmatch(raw, -1) {
		found = append(found, strings.Trim(m[2], "'"))
	}

	out := make([]string, 0, len(found))
	for _, v := range found {
		out = append(out, withDecodedForms(v)...)
	}

	// Longest first, so a secret containing another secret is masked whole.
	sort.SliceStable(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

func userinfoSecrets(raw string) []string {
	_, rest, found := strings.Cut(raw, "://")
	if !found {
		return nil
	}

	if u, err := url.Parse(raw); err == nil && !misparsedUserinfo(u) {
		return parsedPasswordForms(u, rest)
	}

	// net/url could not split the authority; assume everything up to the
	// last '@' is userinfo.
	at := strings.LastIndex(rest, "@")
	if at < 0 {
		return nil
	}
	userinfo := rest[:at]
	if _, password, hasPassword := strings.Cut(userinfo, ":"); hasPassword {
		return []string{password}
	}
	return []string{userinfo}
}

// parsedPasswordForms returns the decoded password of a well-formed URL and
// its raw (percent-encoded) spelling. A username alone is not a credential.
func parsedPasswordForms(u *url.URL, afterScheme string) []string {
	if u.User == nil {
		return nil
	}
	password, hasPassword := u.User.Password()
	if !hasPassword {
		return nil
	}
	out := []string{password}
	authority, _, _ := strings.Cut(afterScheme, "/")
	if at := strings.LastIndex(authority, "@"); at >= 0 {
		if _, rawPassword, ok := strings.Cut(authority[:at], ":"); ok {
			out = append(out, rawPassword)
		}
	}
	return out
}

func querySecrets(raw string) []string {
	_, query, found := strings.Cut(raw, "?")
	if !found {
		return nil
	}
	var out []string
	for _, pair := range strings.Split(query, "&") {
		k, v, _ := strings.Cut(pair, "=")
		if isSensitiveParam(k) {
			out = append(out, v)
		}
	}
	return out
}

// withDecodedForms returns v plus its percent-decoded variants; drivers
// may echo either form.
func withDecodedForms(v string) []string {
	if v == "" {
		return nil
	}
	out := []string{v}
	if decoded, err := url.PathUnescape(v); err == nil && decoded != v {
		out = append(out, decoded)
	}
	if decoded, err := url.QueryUnescape(v); err == nil && decoded != v {
		out = append(out, decoded)
	}
	return out
}

func isSensitiveParam(key string) bool {
	if decoded, err := url.QueryUnescape(key); err == nil {
		key = decoded
	}
	_, ok := sensitiveQueryParams[strings.ToLower(key)]
	return ok
}

func redactQuery(rawQuery string) string {
	pairs := strings.Split(rawQuery, "&")
	for i, pair := range pairs {
		k, _, hasValue := strings.Cut(pair, "=")
		if hasValue && isSensitiveParam(k) {
			pairs[i] = k + "=" + Placeholder
		}
	}
	return strings.Join(pairs, "&")
}

type scrubbedError struct {
	msg string
	err error
}

func (e *scrubbedError) Error() string { return e.msg }

func (e *scrubbedError) Unwrap() error { return e.err }

//nolint:gochecknoglobals // immutable lookup table
var sensitiveHeaders = map[string]struct{}{
	"authorization":        {},
	"proxy-authorization":  {},
	"cookie":               {},
	"set-cookie":           {},
	"x-api-key":            {},
	"x-auth-token":         {},
	"x-csrf-token":         {},
	"x-amz-security-token": {},
}

// HeaderValue masks the value of credential-bearing HTTP headers.
func HeaderValue(name, value string) string {
	if _, ok := sensitiveHeaders[strings.ToLower(name)]; ok {
		return Placeholder
	}
	return value
}

//nolint:gochecknoglobals // immutable lookup table
var sensitiveBodyFields = []string{
	"password", "passwd", "pwd", "secret", "client_secret", "client_assertion",
	"assertion", "access_token", "refresh_token", "id_token", "auth_token", "token",
	"api_key", "apikey", "private_key", "code_verifier", "otp",
}

//nolint:gochecknoglobals // compiled once
var (
	bodyFieldAlternation = strings.Join(sensitiveBodyFields, "|")
	jsonSecretPattern    = regexp.MustCompile(
		`(?i)("(?:` + bodyFieldAlternation + `)"\s*:\s*)"(?:[^"\\]|\\.)*"`)
	formSecretPattern = regexp.MustCompile(
		`(?i)((?:^|[&?])(?:` + bodyFieldAlternation + `)=)[^&\s]*`)
)

// Body masks credential fields (passwords, client secrets/assertions, OAuth
// tokens, ...) in JSON or form-encoded bodies before they are logged.
func Body(body string) string {
	if body == "" {
		return body
	}
	body = jsonSecretPattern.ReplaceAllString(body, `${1}"`+Placeholder+`"`)
	return formSecretPattern.ReplaceAllString(body, "${1}"+Placeholder)
}
