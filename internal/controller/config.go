package controller

import "time"

// Config holds the controller configuration
type Config struct {
	DefaultTimeout time.Duration
	MaxRetries     int
	RetryDelay     time.Duration
	RequeueAfter   time.Duration
}

// DefaultConfig returns the default configuration
func DefaultConfig() Config {
	return Config{
		DefaultTimeout: 10 * time.Second,
		MaxRetries:     3,
		RetryDelay:     1 * time.Second,
		RequeueAfter:   5 * time.Second,
	}
}
