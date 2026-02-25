package version //nolint:revive // package name intentionally matches build-info convention

//nolint:gochecknoglobals //version information is set at build time
var (
	Repository string
	Version    string
	Commit     string
	Date       string
)
