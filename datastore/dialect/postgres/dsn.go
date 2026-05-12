// Package postgres provides the Postgres concrete DialectAdapter.
package postgres

import (
	"fmt"
	"net/url"
	"strings"
)

// NormalizeDSN converts a postgres:// or postgresql:// URI into the
// libpq keyword=value DSN form. If the input already looks like libpq
// form (contains '=' and no postgres scheme prefix) it is returned
// unchanged.
//
// Returns an error if the URI scheme is not postgres / postgresql.
func NormalizeDSN(pgString string) (string, error) {
	trimmed := strings.TrimSpace(pgString)
	lower := strings.ToLower(trimmed)
	if strings.Contains(trimmed, "=") && !strings.HasPrefix(lower, "postgres://") &&
		!strings.HasPrefix(lower, "postgresql://") {
		return trimmed, nil
	}

	return uriToLibpq(trimmed)
}

func uriToLibpq(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}

	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", fmt.Errorf("invalid scheme: %s", u.Scheme)
	}

	user := ""
	password := ""
	if u.User != nil {
		user = u.User.Username()
		password, _ = u.User.Password()
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	dbname := strings.TrimPrefix(u.Path, "/")

	parts := []string{
		"host=" + host,
		"port=" + port,
		"user=" + user,
		"password=" + password,
		"dbname=" + dbname,
	}
	for k, vals := range u.Query() {
		for _, v := range vals {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return strings.Join(parts, " "), nil
}
