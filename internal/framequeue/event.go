package framequeue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// event implements the Event interface
type event struct {
	eventType string
	data      interface{}
}

// NewEvent creates a new event instance
func NewEvent(eventType string, data interface{}) Event {
	return &event{
		eventType: eventType,
		data:      data,
	}
}

// Type returns the event type
func (e *event) Type() string {
	return e.eventType
}

// Data returns the event data
func (e *event) Data() interface{} {
	return e.data
}

// eventRegistry implements the EventRegistry interface
type eventRegistry struct {
	handlers map[string][]EventHandler
	mutex    sync.RWMutex
}

// NewEventRegistry creates a new event registry
func NewEventRegistry() EventRegistry {
	return &eventRegistry{
		handlers: make(map[string][]EventHandler),
	}
}

// GetEventHandlers returns handlers for a specific event type
func (er *eventRegistry) GetEventHandlers(eventType string) []EventHandler {
	er.mutex.RLock()
	defer er.mutex.RUnlock()
	
	handlers, exists := er.handlers[eventType]
	if !exists {
		return nil
	}
	
	// Return a copy to avoid concurrent modification
	result := make([]EventHandler, len(handlers))
	copy(result, handlers)
	return result
}

// HasEvents returns true if any events are registered
func (er *eventRegistry) HasEvents() bool {
	er.mutex.RLock()
	defer er.mutex.RUnlock()
	
	return len(er.handlers) > 0
}

// RegisterHandler registers an event handler for a specific event type
func (er *eventRegistry) RegisterHandler(eventType string, handler EventHandler) {
	er.mutex.Lock()
	defer er.mutex.Unlock()
	
	er.handlers[eventType] = append(er.handlers[eventType], handler)
}

// eventWorker implements SubscribeWorker to handle events from the queue
type eventWorker struct {
	registry     EventRegistry
	workerPool   WorkerPool
	claimsProvider ClaimsProvider
	langProvider LanguageProvider
	logger       Logger
}

// NewEventWorker creates a new event worker
func NewEventWorker(registry EventRegistry, workerPool WorkerPool, claimsProvider ClaimsProvider, langProvider LanguageProvider, logger Logger) SubscribeWorker {
	return &eventWorker{
		registry:       registry,
		workerPool:     workerPool,
		claimsProvider: claimsProvider,
		langProvider:   langProvider,
		logger:         logger,
	}
}

// Handle processes an event message from the queue
func (ew *eventWorker) Handle(ctx context.Context, metadata map[string]string, message []byte) error {
	if ew.registry == nil || !ew.registry.HasEvents() {
		if ew.logger != nil {
			ew.logger.Debug("No event registry or handlers available, skipping message")
		}
		return nil
	}

	// Parse the event from the message
	var eventData struct {
		Type string      `json:"type"`
		Data interface{} `json:"data"`
	}

	if err := json.Unmarshal(message, &eventData); err != nil {
		if ew.logger != nil {
			ew.logger.WithError(err).Error("Failed to unmarshal event message")
		}
		return fmt.Errorf("failed to unmarshal event message: %w", err)
	}

	// Get handlers for this event type
	handlers := ew.registry.GetEventHandlers(eventData.Type)
	if len(handlers) == 0 {
		if ew.logger != nil {
			ew.logger.WithField("eventType", eventData.Type).Debug("No handlers registered for event type")
		}
		return nil
	}

	// Create the event
	event := NewEvent(eventData.Type, eventData.Data)

	// Enhance context with metadata
	enhancedCtx := ew.enhanceContext(ctx, metadata)

	// Process event with each handler
	for _, handler := range handlers {
		if ew.workerPool != nil {
			// Submit as background job if worker pool is available
			job := &eventJob{
				handler: handler,
				event:   event,
				ctx:     enhancedCtx,
				logger:  ew.logger,
			}
			
			if err := ew.workerPool.SubmitJob(enhancedCtx, job); err != nil {
				if ew.logger != nil {
					ew.logger.WithError(err).WithField("eventType", eventData.Type).Error("Failed to submit event job to worker pool")
				}
				// Fall back to direct execution
				if err := handler.Handle(enhancedCtx, event); err != nil {
					if ew.logger != nil {
						ew.logger.WithError(err).WithField("eventType", eventData.Type).Error("Event handler failed")
					}
					return fmt.Errorf("event handler failed for type %s: %w", eventData.Type, err)
				}
			}
		} else {
			// Direct execution
			if err := handler.Handle(enhancedCtx, event); err != nil {
				if ew.logger != nil {
					ew.logger.WithError(err).WithField("eventType", eventData.Type).Error("Event handler failed")
				}
				return fmt.Errorf("event handler failed for type %s: %w", eventData.Type, err)
			}
		}
	}

	if ew.logger != nil {
		ew.logger.WithField("eventType", eventData.Type).WithField("handlerCount", len(handlers)).Debug("Event processed successfully")
	}

	return nil
}

// enhanceContext adds claims and language information to the context
func (ew *eventWorker) enhanceContext(ctx context.Context, metadata map[string]string) context.Context {
	enhancedMetadata := make(map[string]string)
	
	// Copy original metadata
	for k, v := range metadata {
		enhancedMetadata[k] = v
	}
	
	// Add claims metadata if available
	if ew.claimsProvider != nil {
		claimsMetadata := ew.claimsProvider.AsMetadata()
		for k, v := range claimsMetadata {
			enhancedMetadata[k] = v
		}
	}
	
	// Add language metadata if available
	if ew.langProvider != nil {
		// Extract languages from metadata (this would depend on your specific implementation)
		languages := extractLanguagesFromMetadata(metadata)
		langMetadata := ew.langProvider.ToMap(metadata, languages)
		for k, v := range langMetadata {
			enhancedMetadata[k] = v
		}
	}
	
	// In a real implementation, you would add this metadata to the context
	// For now, we'll return the original context
	return ctx
}

// extractLanguagesFromMetadata extracts language preferences from metadata
func extractLanguagesFromMetadata(metadata map[string]string) []string {
	// This is a placeholder implementation
	// In reality, you would extract language preferences from headers like Accept-Language
	if lang, exists := metadata["accept-language"]; exists {
		return []string{lang}
	}
	return []string{"en"} // Default to English
}

// eventJob implements the Job interface for background event processing
type eventJob struct {
	handler EventHandler
	event   Event
	ctx     context.Context
	logger  Logger
}

// Execute executes the event job
func (ej *eventJob) Execute(ctx context.Context) error {
	if err := ej.handler.Handle(ej.ctx, ej.event); err != nil {
		if ej.logger != nil {
			ej.logger.WithError(err).WithField("eventType", ej.event.Type()).Error("Background event job failed")
		}
		return fmt.Errorf("background event job failed for type %s: %w", ej.event.Type(), err)
	}
	
	if ej.logger != nil {
		ej.logger.WithField("eventType", ej.event.Type()).Debug("Background event job completed successfully")
	}
	
	return nil
}
