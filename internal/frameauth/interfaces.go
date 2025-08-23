package frameauth

// ServiceInterface defines the minimal interface needed for authentication
type ServiceInterface interface {
	Config() interface{}
}

// ConfigurationSecurity defines the security configuration interface
type ConfigurationSecurity interface {
	IsRunSecurely() bool
}
