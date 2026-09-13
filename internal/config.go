package heartbeats

import "time"

const (
	// MaxConcurrentReconciles is the number of concurrent reconciliations.
	// This allows multiple Heartbeat resources to be processed in parallel,
	// preventing a single failing endpoint from blocking others.
	MaxConcurrentReconciles = 10
)

// Config tunes HTTP timeouts, retries, and requeue interval.
type Config struct {
	DefaultTimeout time.Duration // Maximum time to wait for HTTP requests
	MaxRetries     int           // Maximum number of retry attempts for failed requests
	RetryDelay     time.Duration // Delay between retry attempts
	RequeueAfter   time.Duration // How often to requeue reconciliation
}

// DefaultConfig returns production timeouts and retry settings.
func DefaultConfig() Config {
	return Config{
		DefaultTimeout: 10 * time.Second,
		MaxRetries:     3,
		RetryDelay:     1 * time.Second,
		RequeueAfter:   5 * time.Second,
	}
}
