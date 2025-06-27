package controller

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1alpha1 "github.com/siutsin/heartbeats/api/v1alpha1"
)

// HealthChecker is an interface for checking endpoint health.
// Implementations should provide a method to check the health of an endpoint given expected status code ranges.
type HealthChecker interface {
	CheckEndpointHealth(
		ctx context.Context,
		endpoint string,
		expectedStatusCodeRanges []monitoringv1alpha1.StatusCodeRange,
		endpointsSecret monitoringv1alpha1.EndpointsSecret,
	) (bool, int, bool, error)
}

// DefaultHealthChecker is the default implementation of HealthChecker.
// It uses HTTP requests and configurable retry logic to check endpoint health.
type DefaultHealthChecker struct {
	config Config
	client *http.Client
}

// NewHealthChecker creates a new DefaultHealthChecker with the provided configuration.
// Returns a HealthChecker interface.
func NewHealthChecker(config Config) HealthChecker {
	return &DefaultHealthChecker{
		config: config,
		client: &http.Client{Timeout: config.DefaultTimeout},
	}
}

// doRequestWithRetries performs the HTTP request with retry logic.
// It attempts the request up to MaxRetries times, handling timeouts and network errors.
// Returns the HTTP response or the last error encountered.
func (h *DefaultHealthChecker) doRequestWithRetries(req *http.Request, log logr.Logger) (*http.Response, error) {
	var lastErr error
	for i := 0; i < h.config.MaxRetries; i++ {
		resp, err := h.client.Do(req)
		if err != nil {
			log.Error(err, "Failed to make request", "endpoint", req.URL.String(), "attempt", i+1)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Error(err, "Request timeout", "timeout", h.config.DefaultTimeout)
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
// Returns:
//
//	bool:  true if the endpoint is considered healthy (status code in expected range), false otherwise.
//	int:   the HTTP status code returned by the endpoint, or 0 if no response was received.
//	bool:  true if the report request succeeded, false otherwise.
//	error: non-nil if an error occurred during the health check (e.g., network error, timeout, invalid request),
//	       or nil if the check was successful.
func (h *DefaultHealthChecker) CheckEndpointHealth(
	ctx context.Context,
	endpoint string,
	expectedStatusCodeRanges []monitoringv1alpha1.StatusCodeRange,
	endpointsSecret monitoringv1alpha1.EndpointsSecret,
) (bool, int, bool, error) {
	log := log.FromContext(ctx)

	// Create request inline
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		log.Error(err, "Failed to create request", "endpoint", endpoint)
		return false, 0, false, fmt.Errorf("%s: %w", ErrFailedToCreateRequest, err)
	}

	resp, err := h.doRequestWithRetries(req, log)
	if err != nil {
		return false, 0, false, err
	}
	if resp != nil {
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Error(err, "Failed to close response body")
			}
		}()
	}

	// Inline status code range check
	healthy := false
	for _, r := range expectedStatusCodeRanges {
		if resp.StatusCode >= r.Min && resp.StatusCode <= r.Max {
			healthy = true
			break
		}
	}

	// Report to the appropriate endpoint
	var reportSuccess bool
	reportURL := endpointsSecret.UnhealthyEndpointKey
	reportMethod := endpointsSecret.UnhealthyEndpointMethod
	if healthy {
		reportURL = endpointsSecret.HealthyEndpointKey
		reportMethod = endpointsSecret.HealthyEndpointMethod
	}
	if reportURL != "" {
		// Default to GET if method is not specified
		if reportMethod == "" {
			reportMethod = "GET"
		}
		reportReq, err := http.NewRequestWithContext(ctx, reportMethod, reportURL, nil)
		if err == nil {
			reportResp, err := h.doRequestWithRetries(reportReq, log)
			if err == nil && reportResp != nil && reportResp.StatusCode >= 200 && reportResp.StatusCode < 300 {
				reportSuccess = true
			}
		} else {
			log.Error(err, "Failed to create report request", "reportURL", reportURL, "method", reportMethod)
		}
	}

	return healthy, resp.StatusCode, reportSuccess, nil
}
