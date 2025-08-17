package framequeue

import (
	"context"
)

// ServiceRegistry defines the interface for service registration needed by queue functionality
type ServiceRegistry interface {
	// RegisterQueueManager registers the queue manager with the service
	RegisterQueueManager(queueManager QueueManager)
	
	// GetConfig returns the service configuration
	GetConfig() Config
	
	// GetLogger returns the service logger
	GetLogger() Logger
	
	// GetWorkerPool returns the worker pool if available
	GetWorkerPool() WorkerPool
	
	// GetEventRegistry returns the event registry if available
	GetEventRegistry() EventRegistry
	
	// GetClaimsProvider returns the claims provider if available
	GetClaimsProvider() ClaimsProvider
	
	// GetLanguageProvider returns the language provider if available
	GetLanguageProvider() LanguageProvider
}

// WithQueue returns an option function that enables queue/pubsub functionality
func WithQueue() func(ctx context.Context, service ServiceRegistry) error {
	return func(ctx context.Context, service ServiceRegistry) error {
		config := service.GetConfig()
		logger := service.GetLogger()
		
		// Create queue manager
		queueManager := NewQueueManager(config, logger)
		
		// Initialize pub/sub system
		if err := queueManager.InitializePubSub(ctx); err != nil {
			if logger != nil {
				logger.WithError(err).Error("Failed to initialize pub/sub system")
			}
			return err
		}
		
		// Register with service
		service.RegisterQueueManager(queueManager)
		
		// Set up event processing if event registry is available
		eventRegistry := service.GetEventRegistry()
		if eventRegistry != nil && eventRegistry.HasEvents() {
			workerPool := service.GetWorkerPool()
			claimsProvider := service.GetClaimsProvider()
			langProvider := service.GetLanguageProvider()
			
			// Create event worker
			eventWorker := NewEventWorker(eventRegistry, workerPool, claimsProvider, langProvider, logger)
			
			// Add event subscriber with the event worker
			eventsQueueName := config.GetEventsQueueName()
			eventsQueueURL := config.GetEventsQueueURL()
			
			if eventsQueueName != "" && eventsQueueURL != "" {
				if err := queueManager.AddSubscriber(ctx, eventsQueueName+"_event_subscriber", eventsQueueURL, eventWorker); err != nil {
					if logger != nil {
						logger.WithError(err).Error("Failed to add event subscriber")
					}
					return err
				}
				
				if logger != nil {
					logger.WithField("eventsQueueName", eventsQueueName).Info("Event processing enabled")
				}
			}
		}
		
		if logger != nil {
			logger.Info("Queue functionality enabled successfully")
		}
		
		return nil
	}
}
