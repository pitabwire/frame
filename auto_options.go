package frame

import (
	"context"

	"github.com/pitabwire/frame/core"
)

// RequireModule ensures a specific module is enabled, returning an error if not configured
func RequireModule(moduleName core.ModuleType) core.Option {
	return func(ctx context.Context, service core.Service) {
		cfg := service.Config()
		detector := core.NewModuleConfigDetector(cfg)
		logger := service.Log(ctx)

		var isEnabled bool
		switch moduleName {
		case core.ModuleTypeAuthentication:
			isEnabled = detector.IsAuthenticationEnabled()
		case core.ModuleTypeAuthorization:
			isEnabled = detector.IsAuthorizationEnabled()
		case core.ModuleTypeData:
			isEnabled = detector.IsDataEnabled()
		case core.ModuleTypeQueue:
			isEnabled = detector.IsQueueEnabled()
		case core.ModuleTypeObservability:
			isEnabled = detector.IsObservabilityEnabled()
		case core.ModuleTypeServer:
			isEnabled = detector.IsServerEnabled()
		default:
			logger.WithField("module", moduleName).Error("Unknown module name")
			return
		}

		if !isEnabled {
			logger.WithField("module", moduleName).Error("Required module is not configured")
			panic("Required module " + moduleName + " is not configured")
		}

		logger.WithField("module", moduleName).Info("Required module is properly configured")
	}
}
