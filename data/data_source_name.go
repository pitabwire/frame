package data

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// Constants for database drivers.
const (
	PostgresScheme = "postgres"
	MysqlScheme    = "mysql://"
	MemScheme      = "mem://"
	RedisScheme    = "redis://"
	NatsScheme     = "nats://"
)

// A DSN for conveniently handling a URI connection string.
type DSN string

func (d DSN) ToArray() []DSN {
	var connectionDSList []DSN
	connectionURIs := strings.Split(string(d), ",")
	for _, connectionURI := range connectionURIs {
		dataSourceURI := DSN(connectionURI)
		if len(dataSourceURI) > 0 {
			connectionDSList = append(connectionDSList, dataSourceURI)
		}
	}

	return connectionDSList
}

func (d DSN) IsMySQL() bool {
	return strings.HasPrefix(string(d), MysqlScheme)
}

func (d DSN) IsPostgres() bool {
	u, err := url.Parse(string(d))

	// Check URI format
	if err == nil && u.Scheme == PostgresScheme {
		return true
	}

	// Check for simple "postgres://" prefix
	if strings.HasPrefix(string(d), PostgresScheme+"://") {
		return true
	}

	// Try parsing again if the first attempt failed but it looks like postgres
	if strings.HasPrefix(string(d), "postgres://") {
		parsedURL, parseErr := url.Parse(string(d))
		if parseErr == nil && parsedURL.Host != "" && parsedURL.Path != "" {
			return true
		}
	}

	// Check for key-value format using regex
	keyValueRegex := regexp.MustCompile(
		`(?i)^(user=\S+|password=\S+|host=\S+|port=\d+|dbname=\S+|sslmode=\S+)(\s+\S+=\S+)*$`,
	)
	return keyValueRegex.MatchString(string(d))
}

func (d DSN) IsDB() bool {
	return d.IsPostgres() || d.IsMySQL()
}

func (d DSN) IsRedis() bool {
	return strings.HasPrefix(string(d), RedisScheme)
}

func (d DSN) IsCache() bool {
	return d.IsRedis()
}

func (d DSN) IsNats() bool {
	return strings.HasPrefix(string(d), NatsScheme)
}

func (d DSN) IsMem() bool {
	return strings.HasPrefix(string(d), MemScheme)
}

func (d DSN) IsQueue() bool {
	return d.IsMem() || d.IsNats()
}

func (d DSN) ToURI() (*url.URL, error) {
	return url.Parse(string(d))
}

func (d DSN) ExtendPath(epath ...string) DSN {
	nuURI, err := d.ToURI()
	if err != nil {
		return d
	}

	nuPathPieces := []string{nuURI.Path}
	nuPathPieces = append(nuPathPieces, epath...)

	nuURI.Path = path.Join(nuPathPieces...)

	return DSN(nuURI.String())
}

func (d DSN) SuffixPath(suffix string) DSN {
	nuURI, err := d.ToURI()
	if err != nil {
		return d
	}

	if nuURI.Path != "" {
		nuURI.Path = strings.Join([]string{nuURI.Path, suffix}, "")
	}
	return DSN(nuURI.String())
}

func (d DSN) PrefixPath(prefix string) DSN {
	nuURI, err := d.ToURI()
	if err != nil {
		return d
	}

	if nuURI.Path != "" {
		nuURI.Path = strings.Join([]string{prefix, nuURI.Path}, "")
	}

	return DSN(nuURI.String())
}

func (d DSN) DelPath() DSN {
	nuURI, err := d.ToURI()
	if err != nil {
		return d
	}

	nuURI.Path = ""

	return DSN(nuURI.String())
}

func (d DSN) ExtendQuery(key, value string) DSN {
	nuURI, err := d.ToURI()
	if err != nil {
		return d
	}

	q := nuURI.Query()
	q.Set(key, value)

	nuURI.RawQuery = q.Encode()

	return DSN(nuURI.String())
}

func (d DSN) RemoveQuery(key ...string) DSN {
	nuURI, err := d.ToURI()
	if err != nil {
		return d
	}

	q := nuURI.Query()

	for _, k := range key {
		q.Del(k)
	}

	nuURI.RawQuery = q.Encode()

	return DSN(nuURI.String())
}

func (d DSN) GetQuery(key string) string {
	nuURI, err := d.ToURI()
	if err != nil {
		return ""
	}

	return nuURI.Query().Get(key)
}

func (d DSN) WithPath(path string) (DSN, error) {
	nuURI, err := d.ToURI()
	if err != nil {
		return "", err
	}

	nuURI.Path = path
	return DSN(nuURI.String()), nil
}

func (d DSN) WithPathSuffix(suffix string) (DSN, error) {
	nuURI, err := d.ToURI()
	if err != nil {
		return "", err
	}

	if nuURI.Path != "" {
		nuURI.Path = strings.Join([]string{nuURI.Path, suffix}, "")
	}
	return DSN(nuURI.String()), nil
}

func (d DSN) WithQuery(query string) (DSN, error) {
	nuURI, err := d.ToURI()
	if err != nil {
		return "", err
	}

	nuURI.RawQuery = query
	return DSN(nuURI.String()), nil
}

func (d DSN) WithUser(user string) (DSN, error) {
	nuURI, err := d.ToURI()
	if err != nil {
		return "", err
	}

	if nuURI.User != nil {
		password, _ := nuURI.User.Password()
		nuURI.User = url.UserPassword(user, password)
	} else {
		nuURI.User = url.User(user)
	}
	return DSN(nuURI.String()), nil
}

func (d DSN) WithPassword(password string) (DSN, error) {
	nuURI, err := d.ToURI()
	if err != nil {
		return "", err
	}

	userName := ""
	if nuURI.User != nil {
		userName = nuURI.User.Username()
	}

	nuURI.User = url.UserPassword(userName, password)
	return DSN(nuURI.String()), nil
}

func (d DSN) WithUserAndPassword(userName, password string) (DSN, error) {
	nuURI, err := d.ToURI()
	if err != nil {
		return "", err
	}

	nuURI.User = url.UserPassword(userName, password)
	return DSN(nuURI.String()), nil
}

func (d DSN) String() string {
	return string(d)
}

func (d DSN) ChangePort(newPort string) (DSN, error) {
	nuURI, err := d.ToURI()
	if err != nil {
		return "", err
	}

	hostname := nuURI.Hostname()
	nuURI.Host = fmt.Sprintf("%s:%s", hostname, newPort)
	return DSN(nuURI.String()), nil
}
