package frame

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

// A DataSource for conveniently handling a URI connection string.
type DataSource string

func (d DataSource) ToArray() []DataSource {
	var connectionDSList []DataSource
	connectionURIs := strings.Split(string(d), ",")
	for _, connectionURI := range connectionURIs {
		dataSourceURI := DataSource(connectionURI)
		if len(dataSourceURI) > 0 {
			connectionDSList = append(connectionDSList, dataSourceURI)
		}
	}

	return connectionDSList
}

func (d DataSource) IsMySQL() bool {
	return strings.HasPrefix(string(d), MysqlScheme)
}

func (d DataSource) IsPostgres() bool {
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

func (d DataSource) IsDB() bool {
	return d.IsPostgres() || d.IsMySQL()
}

func (d DataSource) IsRedis() bool {
	return strings.HasPrefix(string(d), RedisScheme)
}

func (d DataSource) IsCache() bool {
	return d.IsRedis()
}

func (d DataSource) IsNats() bool {
	return strings.HasPrefix(string(d), NatsScheme)
}

func (d DataSource) IsMem() bool {
	return strings.HasPrefix(string(d), MemScheme)
}

func (d DataSource) IsQueue() bool {
	return d.IsMem() || d.IsNats()
}

func (d DataSource) ToURI() (*url.URL, error) {
	return url.Parse(string(d))
}

func (d DataSource) ExtendPath(epath ...string) DataSource {
	nuURI, err := d.ToURI()
	if err != nil {
		return d
	}

	nuPathPieces := []string{nuURI.Path}
	nuPathPieces = append(nuPathPieces, epath...)

	nuURI.Path = path.Join(nuPathPieces...)

	return DataSource(nuURI.String())
}

func (d DataSource) SuffixPath(suffix string) DataSource {
	nuURI, err := d.ToURI()
	if err != nil {
		return d
	}

	if nuURI.Path != "" {
		nuURI.Path = strings.Join([]string{nuURI.Path, suffix}, "")
	}
	return DataSource(nuURI.String())
}

func (d DataSource) PrefixPath(prefix string) DataSource {
	nuURI, err := d.ToURI()
	if err != nil {
		return d
	}

	if nuURI.Path != "" {
		nuURI.Path = strings.Join([]string{prefix, nuURI.Path}, "")
	}

	return DataSource(nuURI.String())
}

func (d DataSource) DelPath() DataSource {
	nuURI, err := d.ToURI()
	if err != nil {
		return d
	}

	nuURI.Path = ""

	return DataSource(nuURI.String())
}

func (d DataSource) ExtendQuery(key, value string) DataSource {
	nuURI, err := d.ToURI()
	if err != nil {
		return d
	}

	q := nuURI.Query()
	q.Set(key, value)

	nuURI.RawQuery = q.Encode()

	return DataSource(nuURI.String())
}

func (d DataSource) RemoveQuery(key ...string) DataSource {
	nuURI, err := d.ToURI()
	if err != nil {
		return d
	}

	q := nuURI.Query()

	for _, k := range key {
		q.Del(k)
	}

	nuURI.RawQuery = q.Encode()

	return DataSource(nuURI.String())
}

func (d DataSource) GetQuery(key string) string {
	nuURI, err := d.ToURI()
	if err != nil {
		return ""
	}

	return nuURI.Query().Get(key)
}

func (d DataSource) WithPath(path string) (DataSource, error) {
	nuURI, err := d.ToURI()
	if err != nil {
		return "", err
	}

	nuURI.Path = path
	return DataSource(nuURI.String()), nil
}

func (d DataSource) WithPathSuffix(suffix string) (DataSource, error) {
	nuURI, err := d.ToURI()
	if err != nil {
		return "", err
	}

	if nuURI.Path != "" {
		nuURI.Path = strings.Join([]string{nuURI.Path, suffix}, "")
	}
	return DataSource(nuURI.String()), nil
}

func (d DataSource) WithQuery(query string) (DataSource, error) {
	nuURI, err := d.ToURI()
	if err != nil {
		return "", err
	}

	nuURI.RawQuery = query
	return DataSource(nuURI.String()), nil
}

func (d DataSource) WithUser(user string) (DataSource, error) {
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
	return DataSource(nuURI.String()), nil
}

func (d DataSource) WithPassword(password string) (DataSource, error) {
	nuURI, err := d.ToURI()
	if err != nil {
		return "", err
	}

	userName := ""
	if nuURI.User != nil {
		userName = nuURI.User.Username()
	}

	nuURI.User = url.UserPassword(userName, password)
	return DataSource(nuURI.String()), nil
}

func (d DataSource) WithUserAndPassword(userName, password string) (DataSource, error) {
	nuURI, err := d.ToURI()
	if err != nil {
		return "", err
	}

	nuURI.User = url.UserPassword(userName, password)
	return DataSource(nuURI.String()), nil
}

func (d DataSource) String() string {
	return string(d)
}

func (d DataSource) ChangePort(newPort string) (DataSource, error) {
	nuURI, err := d.ToURI()
	if err != nil {
		return "", err
	}

	hostname := nuURI.Hostname()
	nuURI.Host = fmt.Sprintf("%s:%s", hostname, newPort)
	return DataSource(nuURI.String()), nil
}
