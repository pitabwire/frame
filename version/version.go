//revive:disable:var-naming // package name intentionally matches build-info convention
package version

//nolint:gochecknoglobals //version information is set at build time
var (
	Repository string
	Version    string
	Commit     string
	Date       string
)
