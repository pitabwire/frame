package frame

import (
	"reflect"
)

// ModuleConfigDetector implements automatic module detection based on configuration values
type ModuleConfigDetector struct {
	config any
}

// NewModuleConfigDetector creates a new module configuration detector
func NewModuleConfigDetector(config any) *ModuleConfigDetector {
	return &ModuleConfigDetector{config: config}
}

// IsAuthenticationEnabled detects if authentication module should be enabled
func (m *ModuleConfigDetector) IsAuthenticationEnabled() bool {
	if m.config == nil {
		return false
	}

	// Check if config implements authentication-related interfaces
	if authConfig, ok := m.config.(ConfigurationOAUTH2); ok {
		// Authentication is enabled if OAuth2 configuration is provided
		return authConfig.GetOauth2WellKnownJwkData() != "" ||
			authConfig.GetOauth2ServiceClientID() != "" ||
			authConfig.GetOauth2ServiceClientSecret() != ""
	}

	// Check for security configuration
	if secConfig, ok := m.config.(interface{ IsRunSecurely() bool }); ok {
		return secConfig.IsRunSecurely()
	}

	return false
}

// IsAuthorizationEnabled detects if authorization module should be enabled
func (m *ModuleConfigDetector) IsAuthorizationEnabled() bool {
	if m.config == nil {
		return false
	}

	// Check if config provides authorization service URIs
	if authzConfig, ok := m.config.(interface {
		GetAuthorizationServiceReadURI() string
		GetAuthorizationServiceWriteURI() string
	}); ok {
		return authzConfig.GetAuthorizationServiceReadURI() != "" ||
			authzConfig.GetAuthorizationServiceWriteURI() != ""
	}

	return false
}

// IsDataEnabled detects if data module should be enabled
func (m *ModuleConfigDetector) IsDataEnabled() bool {
	if m.config == nil {
		return false
	}

	// Check if config implements database configuration
	if dbConfig, ok := m.config.(ConfigurationDatabase); ok {
		urls := dbConfig.GetDatabasePrimaryHostURL()
		return len(urls) > 0 && urls[0] != ""
	}

	// Check for any database-related configuration using reflection
	return m.hasConfigField("DatabaseURL") ||
		m.hasConfigField("DatabaseDriver") ||
		m.hasConfigField("DatabaseMigrationsPath")
}

// IsQueueEnabled detects if queue module should be enabled
func (m *ModuleConfigDetector) IsQueueEnabled() bool {
	if m.config == nil {
		return false
	}

	// Check if config implements queue configuration
	if queueConfig, ok := m.config.(interface {
		GetEventsQueueName() string
		GetEventsQueueURL() string
	}); ok {
		return queueConfig.GetEventsQueueName() != "" ||
			queueConfig.GetEventsQueueURL() != ""
	}

	// Check for any queue-related configuration using reflection
	return m.hasConfigField("EventsQueueName") ||
		m.hasConfigField("EventsQueueURL") ||
		m.hasConfigField("QueueURL")
}

// IsObservabilityEnabled detects if observability module should be enabled
func (m *ModuleConfigDetector) IsObservabilityEnabled() bool {
	if m.config == nil {
		return false
	}

	// Check if config implements tracing configuration
	if tracingConfig, ok := m.config.(interface{ EnableTracing() bool }); ok {
		return tracingConfig.EnableTracing()
	}

	// Check for observability-related configuration using reflection
	return m.hasConfigField("EnableTracing") ||
		m.hasConfigField("TracingEndpoint") ||
		m.hasConfigField("MetricsEndpoint") ||
		m.hasConfigField("LoggingLevel")
}

// IsServerEnabled detects if server module should be enabled
func (m *ModuleConfigDetector) IsServerEnabled() bool {
	if m.config == nil {
		return true // Server is enabled by default
	}

	// Check if config implements port configuration
	if portConfig, ok := m.config.(ConfigurationPorts); ok {
		return portConfig.HTTPPort() != "" || portConfig.GrpcPort() != ""
	}

	// Server is enabled by default unless explicitly disabled
	return true
}

// hasConfigField checks if the configuration has a specific field using reflection
func (m *ModuleConfigDetector) hasConfigField(fieldName string) bool {
	if m.config == nil {
		return false
	}

	configValue := reflect.ValueOf(m.config)
	if configValue.Kind() == reflect.Ptr {
		configValue = configValue.Elem()
	}

	if configValue.Kind() != reflect.Struct {
		return false
	}

	field := configValue.FieldByName(fieldName)
	if !field.IsValid() {
		return false
	}

	// Check if field has a non-zero value
	switch field.Kind() {
	case reflect.String:
		return field.String() != ""
	case reflect.Bool:
		return field.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return field.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return field.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return field.Float() != 0
	default:
		return !field.IsZero()
	}
}

// GetEnabledModules returns a list of modules that should be enabled based on configuration
func (m *ModuleConfigDetector) GetEnabledModules() []string {
	var modules []string

	if m.IsAuthenticationEnabled() {
		modules = append(modules, "authentication")
	}
	if m.IsAuthorizationEnabled() {
		modules = append(modules, "authorization")
	}
	if m.IsDataEnabled() {
		modules = append(modules, "data")
	}
	if m.IsQueueEnabled() {
		modules = append(modules, "queue")
	}
	if m.IsObservabilityEnabled() {
		modules = append(modules, "observability")
	}
	if m.IsServerEnabled() {
		modules = append(modules, "server")
	}

	return modules
}
