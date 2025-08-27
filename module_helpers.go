package frame

import (
	"github.com/pitabwire/frame/modules/authentication"
	"github.com/pitabwire/frame/modules/authorization"
	"github.com/pitabwire/frame/modules/data"
	"github.com/pitabwire/frame/modules/queue"
	"github.com/pitabwire/frame/modules/observability"
	"github.com/pitabwire/frame/modules/server"
)

// Helper functions for easy module retrieval by end users
// These provide type-safe access to specific module implementations

// GetAuthenticator retrieves the authentication module from the service
func GetAuthenticator(service Service) authentication.Authenticator {
	module := service.GetModule(ModuleTypeAuthentication)
	if module == nil {
		return nil
	}
	
	if authModule, ok := module.(authentication.Module); ok {
		return authModule.Authenticator()
	}
	
	return nil
}

// GetAuthorizer retrieves the authorization module from the service
func GetAuthorizer(service Service) authorization.Authorizer {
	module := service.GetModule(ModuleTypeAuthorization)
	if module == nil {
		return nil
	}
	
	if authzModule, ok := module.(authorization.Module); ok {
		return authzModule.Authorizer()
	}
	
	return nil
}

// GetDatastoreManager retrieves the datastore manager from the service
func GetDatastoreManager(service Service) data.DatastoreManager {
	module := service.GetModule(ModuleTypeData)
	if module == nil {
		return nil
	}
	
	if dataModule, ok := module.(data.Module); ok {
		return dataModule.DatastoreManager()
	}
	
	return nil
}

// GetMigrator retrieves the migrator from the service
func GetMigrator(service Service) data.Migrator {
	module := service.GetModule(ModuleTypeData)
	if module == nil {
		return nil
	}
	
	if dataModule, ok := module.(data.Module); ok {
		return dataModule.Migrator()
	}
	
	return nil
}

// GetSearchProvider retrieves the search provider from the service
func GetSearchProvider(service Service) data.SearchProvider {
	module := service.GetModule(ModuleTypeData)
	if module == nil {
		return nil
	}
	
	if dataModule, ok := module.(data.Module); ok {
		return dataModule.SearchProvider()
	}
	
	return nil
}

// GetQueueManager retrieves the queue manager from the service
func GetQueueManager(service Service) queue.QueueManager {
	module := service.GetModule(ModuleTypeQueue)
	if module == nil {
		return nil
	}
	
	if queueModule, ok := module.(queue.Module); ok {
		return queueModule.QueueManager()
	}
	
	return nil
}

// GetObservabilityManager retrieves the observability manager from the service
func GetObservabilityManager(service Service) observability.ObservabilityManager {
	module := service.GetModule(ModuleTypeObservability)
	if module == nil {
		return nil
	}
	
	if obsModule, ok := module.(observability.Module); ok {
		return obsModule.ObservabilityManager()
	}
	
	return nil
}

// GetServerManager retrieves the server manager from the service
func GetServerManager(service Service) server.ServerManager {
	module := service.GetModule(ModuleTypeServer)
	if module == nil {
		return nil
	}
	
	if serverModule, ok := module.(server.Module); ok {
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