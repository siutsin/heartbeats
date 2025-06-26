package controller

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	monitoringv1alpha1 "github.com/siutsin/heartbeats/api/v1alpha1"
)

// HealthChecker is an interface for checking endpoint health.
// Implementations should provide a method to check the health of an endpoint given expected status code ranges.
type HealthChecker interface {
	CheckEndpointHealth(
		ctx context.Context,
		endpoint string,
		expectedStatusCodeRanges []monitoringv1alpha1.StatusCodeRange,
	) (bool, int, error)
}

// DefaultHealthChecker is the default implementation of HealthChecker.
// It uses HTTP requests and configurable retry logic to check endpoint health.
type DefaultHealthChecker struct {
	config Config
}

// NewHealthChecker creates a new DefaultHealthChecker with the provided configuration.
// Returns a HealthChecker interface.
func NewHealthChecker(config Config) HealthChecker {
	return &DefaultHealthChecker{
		config: config,
	}
}

// createRequest creates a new HTTP GET request with the provided context and endpoint URL.
// Returns the constructed *http.Request or an error if the request could not be created.
func createRequest(ctx context.Context, endpoint string) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
}

// isStatusCodeInRange checks if the given status code is within any of the provided expected ranges.
// Returns true if the status code is in range, false otherwise.
func isStatusCodeInRange(statusCode int, ranges []monitoringv1alpha1.StatusCodeRange) bool {
	for _, r := range ranges {
		if statusCode >= r.Min && statusCode <= r.Max {
			return true
		}
	}
	return false
}

// doRequestWithRetries performs the HTTP request with retry logic.
// It attempts the request up to MaxRetries times, handling timeouts and network errors.
// Returns the HTTP response or the last error encountered.
func (h *DefaultHealthChecker) doRequestWithRetries(req *http.Request, log logr.Logger) (*http.Response, error) {
	var lastErr error
	client := &http.Client{Timeout: h.config.DefaultTimeout}
	for i := 0; i < h.config.MaxRetries; i++ {
		resp, err := client.Do(req)
		if err != nil {
			log.Error(err, "Failed to make request", "endpoint", req.URL.String(), "attempt", i+1)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Error(err, ErrEndpointTimeout, "timeout", h.config.DefaultTimeout)
				lastErr = fmt.Errorf("%s: %w", ErrEndpointTimeout, err)
			} else {
				lastErr = fmt.Errorf("%s: %w", ErrFailedToMakeRequest, err)
			}
			time.Sleep(h.config.RetryDelay)
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

// CheckEndpointHealth checks if the endpoint is healthy by making an HTTP request.
//
// Returns:
//
//	bool:  true if the endpoint is considered healthy (status code in expected range), false otherwise.
//	int:   the HTTP status code returned by the endpoint, or 0 if no response was received.
//	error: non-nil if an error occurred during the health check (e.g., network error, timeout, invalid request),
//	       or nil if the check was successful.
//
// This allows the caller to distinguish between healthy/unhealthy endpoints, inspect the actual status code,
// and handle errors in an idiomatic Go way.
func (h *DefaultHealthChecker) CheckEndpointHealth(
	ctx context.Context,
	endpoint string,
	expectedStatusCodeRanges []monitoringv1alpha1.StatusCodeRange) (bool, int, error) {
	log := log.FromContext(ctx)

	req, err := createRequest(ctx, endpoint)
	if err != nil {
		log.Error(err, ErrFailedToCreateRequest, "endpoint", endpoint)
		return false, 0, fmt.Errorf("%s: %w", ErrFailedToCreateRequest, err)
	}

	resp, err := h.doRequestWithRetries(req, log)
	if err != nil {
		return false, 0, err
	}
	if resp != nil {
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Error(err, "Failed to close response body")
			}
		}()
	}

	if isStatusCodeInRange(resp.StatusCode, expectedStatusCodeRanges) {
		return true, resp.StatusCode, nil
	}
	return false, resp.StatusCode, nil
}
