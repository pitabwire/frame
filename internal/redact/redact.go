// Package redact removes credentials from URLs, DSNs, error messages and
// headers before they are logged or returned to callers.
package redact

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// Placeholder replaces credential material in redacted output.
const Placeholder = "xxxxx"

//nolint:gochecknoglobals // immutable lookup table
var sensitiveQueryParams = map[string]struct{}{
	"password":         {},
	"pass":             {},
	"passwd":           {},
	"pwd":              {},
	"secret":           {},
	"client_secret":    {},
	"token":            {},
	"access_token":     {},
	"refresh_token":    {},
	"auth_token":       {},
	"api_key":          {},
	"apikey":           {},
	"key":              {},
	"sig":              {},
	"signature":        {},
	"x-amz-signature":  {},
	"x-amz-credential": {},
	"credentials":      {},
	"nkey":             {},
	"jwt":              {},
}

// keywordPasswordPattern matches password=value in libpq keyword/value DSNs.
var keywordPasswordPattern = regexp.MustCompile(`(?i)(\bpassword\s*=\s*)('(?:[^'\\]|\\.)*'|\S+)`)

// URL returns raw with credentials removed so it is safe to log or
// embed in errors: the userinfo password (or a token-only userinfo) and
// sensitive query parameters are replaced with Placeholder, as is the
// password of libpq keyword/value DSNs. Strings that net/url cannot parse
// still have their authority userinfo masked.
func URL(raw string) string {
	if raw == "" {
		return raw
	}

	if !strings.Contains(raw, "://") {
		return keywordPasswordPattern.ReplaceAllString(raw, "${1}"+Placeholder)
	}

	u, err := url.Parse(raw)
	if err != nil {
		return redactUnparsedUserinfo(raw)
	}

	if u.User != nil {
		if _, hasPassword := u.User.Password(); hasPassword {
			u.User = url.UserPassword(u.User.Username(), Placeholder)
		} else if u.User.Username() != "" {
			// A bare userinfo is commonly a token (redis://token@host).
			u.User = url.User(Placeholder)
		}
	}

	if u.RawQuery != "" {
		u.RawQuery = redactQuery(u.RawQuery)
	}

	return u.String()
}

// Error returns err with every credential of the raw URL/DSN removed from its
// message. Drivers and net/url routinely echo the full connection URL in
// errors, so errors derived from a DSN must be scrubbed before they are
// logged or surfaced. The original error stays reachable through Unwrap for
// errors.Is / errors.As. Errors that contain no credential are returned as-is.
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

// Text removes every credential of the raw URL/DSN from text.
func Text(raw, text string) string {
	if raw == "" || text == "" {
		return text
	}

	text = strings.ReplaceAll(text, raw, URL(raw))
	for _, secret := range secrets(raw) {
		text = strings.ReplaceAll(text, secret, Placeholder)
	}
	return text
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
	userinfo, ok := authorityUserinfo(raw)
	if !ok {
		return nil
	}
	if _, password, hasPassword := strings.Cut(userinfo, ":"); hasPassword {
		return []string{password}
	}
	return []string{userinfo}
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

// authorityUserinfo extracts the userinfo of scheme://userinfo@host without
// relying on net/url, so it also works for strings net/url rejects.
func authorityUserinfo(raw string) (string, bool) {
	_, rest, found := strings.Cut(raw, "://")
	if !found {
		return "", false
	}
	if end := strings.IndexAny(rest, "/?#"); end >= 0 {
		rest = rest[:end]
	}
	at := strings.LastIndex(rest, "@")
	if at < 0 {
		return "", false
	}
	return rest[:at], true
}

func redactUnparsedUserinfo(raw string) string {
	userinfo, ok := authorityUserinfo(raw)
	if !ok {
		return raw
	}
	scheme, rest, _ := strings.Cut(raw, "://")
	replacement := Placeholder
	if user, _, found := strings.Cut(userinfo, ":"); found {
		replacement = user + ":" + Placeholder
	}
	return scheme + "://" + replacement + strings.TrimPrefix(rest, userinfo)
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
