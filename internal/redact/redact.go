// Package redact removes credentials from URLs, DSNs, error messages and
// headers before they are logged or returned to callers.
package redact

import (
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Placeholder replaces credential material in redacted output.
const Placeholder = "xxxxx"

//nolint:gochecknoglobals // immutable lookup table
var sensitiveQueryParams = map[string]struct{}{
	"password":             {},
	"sslpassword":          {},
	"id_token":             {},
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
var keywordPasswordPattern = regexp.MustCompile(`(?i)(\b(?:ssl)?password\s*=\s*)('(?:[^'\\]|\\.)*'|\S+)`)

// URL returns raw with credentials removed so it is safe to log or embed
// in errors: the userinfo password and sensitive query parameters are
// replaced with Placeholder, as is the password of libpq keyword/value DSNs.
// A bare userinfo without a password is masked too, since it is commonly a
// token (redis://<token>@host, nats://<token>@host).
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
	if err != nil || misSplit(raw) {
		return maskUpToLastAt(raw)
	}

	if u.User != nil {
		if _, hasPassword := u.User.Password(); hasPassword {
			u.User = url.UserPassword(u.User.Username(), Placeholder)
		} else if u.User.Username() != "" {
			// A bare userinfo is commonly the credential itself
			// (redis://<token>@host, nats://<token>@host).
			u.User = url.User(Placeholder)
		}
	}

	if u.RawQuery != "" {
		u.RawQuery = redactQuery(u.RawQuery)
	}
	if u.Fragment != "" {
		// OAuth implicit/hybrid flows return tokens in the fragment.
		u.Fragment = redactQuery(u.Fragment)
		u.RawFragment = ""
	}

	return u.String()
}

// misSplit reports whether an '@' appears after the first '/', '?' or '#'
// that follows "://". net/url ends the authority at that delimiter, so such
// an '@' means a password or token containing a reserved character was
// split into path/query/fragment (e.g. user:p/ss@host, tok#en@host). The
// whole span up to the last '@' is then treated as userinfo. This may
// over-redact legitimate URLs with '@' in the path or query, which is the
// accepted trade-off.
func misSplit(raw string) bool {
	_, rest, found := strings.Cut(raw, "://")
	if !found {
		return false
	}
	delim := strings.IndexAny(rest, "/?#")
	return delim >= 0 && strings.Contains(rest[delim:], "@")
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
	if q := strings.IndexAny(tail, "?#"); q >= 0 {
		tail = tail[:q+1] + redactParams(tail[q+1:])
	}
	return scheme + "://" + prefix + tail
}

// redactParams masks sensitive key=value pairs separated by '&', '?' or '#'.
func redactParams(s string) string {
	var b strings.Builder
	start := 0
	for i := 0; i <= len(s); i++ {
		if i < len(s) && s[i] != '&' && s[i] != '?' && s[i] != '#' {
			continue
		}
		b.WriteString(redactQuery(s[start:i]))
		if i < len(s) {
			b.WriteByte(s[i])
		}
		start = i + 1
	}
	return b.String()
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

	if u, err := url.Parse(raw); err == nil && !misSplit(raw) {
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

// parsedPasswordForms returns the userinfo credential of a well-formed URL:
// the password (decoded and raw spellings), or the bare username when there
// is no password, since that is commonly a token.
func parsedPasswordForms(u *url.URL, afterScheme string) []string {
	if u.User == nil {
		return nil
	}
	authority, _, _ := strings.Cut(afterScheme, "/")
	rawUserinfo := ""
	if at := strings.LastIndex(authority, "@"); at >= 0 {
		rawUserinfo = authority[:at]
	}

	password, hasPassword := u.User.Password()
	if !hasPassword {
		return []string{u.User.Username(), rawUserinfo}
	}
	out := []string{password}
	if _, rawPassword, ok := strings.Cut(rawUserinfo, ":"); ok {
		out = append(out, rawPassword)
	}
	return out
}

func querySecrets(raw string) []string {
	start := strings.IndexAny(raw, "?#")
	if start < 0 {
		return nil
	}
	var out []string
	for _, pair := range strings.FieldsFunc(raw[start+1:], func(r rune) bool {
		return r == '&' || r == '?' || r == '#'
	}) {
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

// Body masks credential fields in JSON or form-encoded bodies before they
// are logged: passwords, client secrets and assertions, OAuth tokens, API
// keys, OTPs, and (form-encoded only) the OAuth authorization "code". Keys
// match case-insensitively and regardless of '_' / '-' separators
// (clientSecret, client_secret, new-password). String, numeric, boolean,
// array and object values are all masked, as is a value cut off by a
// truncated body.
func Body(body string) string {
	if body == "" {
		return body
	}
	return redactFormBody(redactJSONBody(body))
}

// jsonKeyPattern finds `"key"` followed by ':'.
var jsonKeyPattern = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"\s*:\s*`)

// formPairPattern finds key=value pairs at the start of the body or after
// '&', '?' or whitespace.
var formPairPattern = regexp.MustCompile(`(^|[&?\s])([A-Za-z0-9_.\-]+)=([^&\s]*)`)

func redactJSONBody(body string) string {
	var b strings.Builder
	last := 0
	for _, m := range jsonKeyPattern.FindAllStringSubmatchIndex(body, -1) {
		keyStart, valueStart := m[0], m[1]
		if keyStart < last {
			continue // inside a value we already replaced
		}
		if !isSensitiveBodyKey(body[m[2]:m[3]], false) {
			continue
		}
		end := jsonValueEnd(body, valueStart)
		b.WriteString(body[last:valueStart])
		b.WriteString(`"` + Placeholder + `"`)
		last = end
	}
	if last == 0 {
		return body
	}
	b.WriteString(body[last:])
	return b.String()
}

// jsonValueEnd returns the index just past the JSON value starting at i,
// or len(s) when the value is truncated.
func jsonValueEnd(s string, i int) int {
	if i >= len(s) {
		return len(s)
	}
	switch s[i] {
	case '"':
		return jsonStringEnd(s, i)
	case '[', '{':
		return jsonContainerEnd(s, i)
	default:
		j := i
		for j < len(s) && !strings.ContainsRune(",}] \t\r\n", rune(s[j])) {
			j++
		}
		return j
	}
}

// jsonStringEnd returns the index just past the string starting at s[i]=='"'.
func jsonStringEnd(s string, i int) int {
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '"':
			return j + 1
		}
	}
	return len(s)
}

// jsonContainerEnd returns the index just past the array/object starting at
// s[i], skipping brackets inside strings.
func jsonContainerEnd(s string, i int) int {
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '"':
			j = jsonStringEnd(s, j) - 1
		case '[', '{':
			depth++
		case ']', '}':
			depth--
			if depth == 0 {
				return j + 1
			}
		}
	}
	return len(s)
}

func redactFormBody(body string) string {
	return formPairPattern.ReplaceAllStringFunc(body, func(pair string) string {
		m := formPairPattern.FindStringSubmatch(pair)
		if !isSensitiveBodyKey(m[2], true) {
			return pair
		}
		return m[1] + m[2] + "=" + Placeholder
	})
}

//nolint:gochecknoglobals // immutable lookup tables
var (
	// sensitiveKeySuffixes match normalised keys ending in them
	// (newpassword, clientsecret, refreshtoken, xapikey, ...).
	sensitiveKeySuffixes = []string{
		"password", "passwd", "secret", "token", "apikey", "privatekey", "assertion", "credential", "credentials",
	}
	// sensitiveExactKeys match whole normalised keys only.
	sensitiveExactKeys = map[string]struct{}{
		"pwd": {}, "pass": {}, "otp": {}, "pin": {}, "cvv": {}, "codeverifier": {}, "sig": {}, "signature": {},
	}
	// benignTokenKeys end in "token"-like suffixes but carry no secret.
	benignKeys = map[string]struct{}{
		"tokentype": {}, "tokenendpoint": {}, "nexttoken": {},
	}
)

// isSensitiveBodyKey normalises key (lower case, no '_', '-' or '.') and
// reports whether it names a credential. The OAuth authorization "code" is
// only treated as secret in form bodies, where it is a grant parameter;
// JSON "code" fields are usually status codes.
func isSensitiveBodyKey(key string, form bool) bool {
	norm := strings.Map(func(r rune) rune {
		switch r {
		case '_', '-', '.':
			return -1
		}
		return unicode.ToLower(r)
	}, key)
	if _, ok := benignKeys[norm]; ok {
		return false
	}
	if form && norm == "code" {
		return true
	}
	if _, ok := sensitiveExactKeys[norm]; ok {
		return true
	}
	for _, suffix := range sensitiveKeySuffixes {
		if strings.HasSuffix(norm, suffix) {
			return true
		}
	}
	return false
}
