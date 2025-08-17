package framedata

import (
	"context"
	"sync"
	"time"
)

// healthChecker implements the HealthChecker interface
type healthChecker struct {
	datastore DatastoreManager
	logger    Logger
	
	// Health state
	healthy       bool
	lastCheck     time.Time
	lastError     string
	responseTime  time.Duration
	
	// Monitoring control
	stopChan chan struct{}
	running  bool
	mutex    sync.RWMutex
}

// NewHealthChecker creates a new health checker instance
func NewHealthChecker(datastore DatastoreManager, logger Logger) HealthChecker {
	return &healthChecker{
		datastore: datastore,
		logger:    logger,
		healthy:   true, // Assume healthy initially
		stopChan:  make(chan struct{}),
	}
}

// CheckHealth performs a health check on the datastore
func (hc *healthChecker) CheckHealth(ctx context.Context) HealthStatus {
	start := time.Now()
	
	hc.mutex.Lock()
	defer hc.mutex.Unlock()
	
	hc.lastCheck = start
	
	// Check write connection
	writeDB, err := hc.datastore.GetConnection(ctx)
	if err != nil {
		hc.healthy = false
		hc.lastError = err.Error()
		hc.responseTime = time.Since(start)
		
		if hc.logger != nil {
			hc.logger.WithError(err).Error("Health check failed: unable to get write connection")
		}
		
		return hc.buildHealthStatus()
	}
	
	// Ping write database
	if err := writeDB.PingContext(ctx); err != nil {
		hc.healthy = false
		hc.lastError = err.Error()
		hc.responseTime = time.Since(start)
		
		if hc.logger != nil {
			hc.logger.WithError(err).Error("Health check failed: write database ping failed")
		}
		
		return hc.buildHealthStatus()
	}
	
	// Check read connection if available
	readDB, err := hc.datastore.GetReadOnlyConnection(ctx)
	if err == nil && readDB != writeDB {
		if err := readDB.PingContext(ctx); err != nil {
			hc.healthy = false
			hc.lastError = err.Error()
			hc.responseTime = time.Since(start)
			
			if hc.logger != nil {
				hc.logger.WithError(err).Error("Health check failed: read database ping failed")
			}
			
			return hc.buildHealthStatus()
		}
	}
	
	// All checks passed
	hc.healthy = true
	hc.lastError = ""
	hc.responseTime = time.Since(start)
	
	if hc.logger != nil {
		hc.logger.WithField("responseTime", hc.responseTime).Debug("Health check passed")
	}
	
	return hc.buildHealthStatus()
}

// StartHealthMonitoring starts periodic health monitoring
func (hc *healthChecker) StartHealthMonitoring(ctx context.Context, interval time.Duration) error {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()
	
	if hc.running {
		return nil // Already running
	}
	
	hc.running = true
	hc.stopChan = make(chan struct{})
	
	go hc.monitorHealth(ctx, interval)
	
	if hc.logger != nil {
		hc.logger.WithField("interval", interval).Info("Health monitoring started")
	}
	
	return nil
}

// StopHealthMonitoring stops health monitoring
func (hc *healthChecker) StopHealthMonitoring() {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()
	
	if !hc.running {
		return
	}
	
	close(hc.stopChan)
	hc.running = false
	
	if hc.logger != nil {
		hc.logger.Info("Health monitoring stopped")
	}
}

// IsHealthy returns the current health status
func (hc *healthChecker) IsHealthy() bool {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()
	
	return hc.healthy
}

// GetLastHealthCheck returns the time of the last health check
func (hc *healthChecker) GetLastHealthCheck() time.Time {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()
	
	return hc.lastCheck
}

// monitorHealth runs the health monitoring loop
func (hc *healthChecker) monitorHealth(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-hc.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Perform health check with timeout
			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			hc.CheckHealth(checkCtx)
			cancel()
		}
	}
}

// buildHealthStatus builds a HealthStatus from current state
func (hc *healthChecker) buildHealthStatus() HealthStatus {
	return HealthStatus{
		Healthy:        hc.healthy,
		LastCheck:      hc.lastCheck,
		ResponseTime:   hc.responseTime,
		Error:          hc.lastError,
		ConnectionPool: hc.datastore.GetConnectionPoolStats(),
	}
}
