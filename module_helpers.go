package frame

import (
	"github.com/pitabwire/frame/internal/frameauth"
	"github.com/pitabwire/frame/internal/frameauthorization"
	"github.com/pitabwire/frame/internal/framedata"
	"github.com/pitabwire/frame/internal/framequeue"
	"github.com/pitabwire/frame/internal/frameobservability"
	"github.com/pitabwire/frame/internal/frameserver"
)

// Helper functions for easy module retrieval by end users
// These provide type-safe access to specific module implementations

// GetAuthenticator retrieves the authentication module from the service
func GetAuthenticator(service Service) frameauth.Authenticator {
	module := service.GetModule(ModuleTypeAuthentication)
	if module == nil {
		return nil
	}
	
	if authModule, ok := module.(interface{ Authenticator() frameauth.Authenticator }); ok {
		return authModule.Authenticator()
	}
	
	return nil
}

// GetAuthorizer retrieves the authorization module from the service
func GetAuthorizer(service Service) frameauthorization.Authorizer {
	module := service.GetModule(ModuleTypeAuthorization)
	if module == nil {
		return nil
	}
	
	if authzModule, ok := module.(interface{ Authorizer() frameauthorization.Authorizer }); ok {
		return authzModule.Authorizer()
	}
	
	return nil
}

// GetDatastoreManager retrieves the datastore manager from the service
func GetDatastoreManager(service Service) framedata.DatastoreManager {
	module := service.GetModule(ModuleTypeData)
	if module == nil {
		return nil
	}
	
	if dataModule, ok := module.(interface{ DatastoreManager() framedata.DatastoreManager }); ok {
		return dataModule.DatastoreManager()
	}
	
	return nil
}

// GetMigrator retrieves the migrator from the service
func GetMigrator(service Service) framedata.Migrator {
	module := service.GetModule(ModuleTypeData)
	if module == nil {
		return nil
	}
	
	if dataModule, ok := module.(interface{ Migrator() framedata.Migrator }); ok {
		return dataModule.Migrator()
	}
	
	return nil
}

// GetSearchProvider retrieves the search provider from the service
func GetSearchProvider(service Service) framedata.SearchProvider {
	module := service.GetModule(ModuleTypeData)
	if module == nil {
		return nil
	}
	
	if dataModule, ok := module.(interface{ SearchProvider() framedata.SearchProvider }); ok {
		return dataModule.SearchProvider()
	}
	
	return nil
}

// GetQueueManager retrieves the queue manager from the service
func GetQueueManager(service Service) framequeue.QueueManager {
	module := service.GetModule(ModuleTypeQueue)
	if module == nil {
		return nil
	}
	
	if queueModule, ok := module.(interface{ QueueManager() framequeue.QueueManager }); ok {
		return queueModule.QueueManager()
	}
	
	return nil
}

// GetObservabilityManager retrieves the observability manager from the service
func GetObservabilityManager(service Service) frameobservability.ObservabilityManager {
	module := service.GetModule(ModuleTypeObservability)
	if module == nil {
		return nil
	}
	
	if obsModule, ok := module.(interface{ ObservabilityManager() frameobservability.ObservabilityManager }); ok {
		return obsModule.ObservabilityManager()
	}
	
	return nil
}

// GetServerManager retrieves the server manager from the service
func GetServerManager(service Service) frameserver.ServerManager {
	module := service.GetModule(ModuleTypeServer)
	if module == nil {
		return nil
	}
	
	if serverModule, ok := module.(interface{ ServerManager() frameserver.ServerManager }); ok {
		return serverModule.ServerManager()
	}
	
	return nil
}

// IsModuleEnabled checks if a specific module type is enabled
func IsModuleEnabled(service Service, moduleType ModuleType) bool {
	return service.HasModule(moduleType)
}

// ListEnabledModules returns a list of all enabled module types
func ListEnabledModules(service Service) []ModuleType {
	var enabled []ModuleType
	for _, moduleType := range service.Modules().List() {
		if service.HasModule(moduleType) {
			enabled = append(enabled, moduleType)
		}
	}
	return enabled
}
