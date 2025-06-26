package controller

import "time"

// Config holds the controller configuration parameters.
// It defines timeouts, retry settings, and requeue intervals for the heartbeat controller.
type Config struct {
	DefaultTimeout time.Duration // Maximum time to wait for HTTP requests
	MaxRetries     int           // Maximum number of retry attempts for failed requests
	RetryDelay     time.Duration // Delay between retry attempts
	RequeueAfter   time.Duration // How often to requeue reconciliation
}

// DefaultConfig returns the default configuration for the heartbeat controller.
// This provides sensible defaults for production use.
//
// Returns:
//   - Config: A configuration struct with default values:
//   - DefaultTimeout: 10 seconds for HTTP requests
//   - MaxRetries: 3 retry attempts
//   - RetryDelay: 1 second between retries
//   - RequeueAfter: 5 seconds between reconciliations
func DefaultConfig() Config {
	return Config{
		DefaultTimeout: 10 * time.Second,
		MaxRetries:     3,
		RetryDelay:     1 * time.Second,
		RequeueAfter:   5 * time.Second,
	}
}
