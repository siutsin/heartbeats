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
		h.logRequestStart(req, log, i+1)

		resp, err := h.executeRequest(req, log, i+1)
		if err != nil {
			lastErr = h.handleRequestError(req, log, err, i+1)
			h.handleRetry(req, log, i)
			continue
		}

		h.logSuccessfulRequest(req, log, resp, i+1)
		return resp, nil
	}

	h.logFinalFailure(req, log, lastErr)
	return nil, lastErr
}

// logRequestStart logs the start of an HTTP request attempt.
func (h *DefaultHealthChecker) logRequestStart(req *http.Request, log logr.Logger, attempt int) {
	log.V(1).Info("Starting HTTP request",
		"method", req.Method,
		"endpoint", req.URL.String(),
		"attempt", attempt,
		"max_attempts", h.config.MaxRetries,
		"timeout", h.config.DefaultTimeout,
	)
}

// executeRequest performs a single HTTP request and measures its duration.
func (h *DefaultHealthChecker) executeRequest(req *http.Request, log logr.Logger, attempt int) (*http.Response, error) {
	startTime := time.Now()
	resp, err := h.client.Do(req)
	duration := time.Since(startTime)

	if err != nil {
		log.Error(err, "HTTP request failed",
			"method", req.Method,
			"endpoint", req.URL.String(),
			"attempt", attempt,
			"duration_ms", duration.Milliseconds(),
		)
	}

	return resp, err
}

// handleRequestError processes and categorises request errors.
func (h *DefaultHealthChecker) handleRequestError(req *http.Request, log logr.Logger, err error, attempt int) error {
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		log.Error(err, "Request timeout",
			"method", req.Method,
			"endpoint", req.URL.String(),
			"timeout", h.config.DefaultTimeout,
			"attempt", attempt,
		)
		return fmt.Errorf("%s: %w", ErrEndpointTimeout, err)
	}

	log.Error(err, "Network error",
		"method", req.Method,
		"endpoint", req.URL.String(),
		"attempt", attempt,
	)
	return fmt.Errorf("%s: %w", ErrFailedToMakeRequest, err)
}

// handleRetry manages retry logic and delays between attempts.
func (h *DefaultHealthChecker) handleRetry(req *http.Request, log logr.Logger, attemptIndex int) {
	if attemptIndex < h.config.MaxRetries-1 {
		log.V(1).Info("Retrying request",
			"method", req.Method,
			"endpoint", req.URL.String(),
			"attempt", attemptIndex+2,
			"timeout", h.config.RetryDelay,
		)
		time.Sleep(h.config.RetryDelay)
	}
}

// logSuccessfulRequest logs details of a successful HTTP request.
func (h *DefaultHealthChecker) logSuccessfulRequest(
	req *http.Request,
	log logr.Logger,
	resp *http.Response,
	attempt int,
) {
	log.V(1).Info("HTTP request successful",
		"method", req.Method,
		"endpoint", req.URL.String(),
		"status_code", resp.StatusCode,
		"attempt", attempt,
	)
}

// logFinalFailure logs the final failure after all retry attempts are exhausted.
func (h *DefaultHealthChecker) logFinalFailure(req *http.Request, log logr.Logger, lastErr error) {
	log.Error(lastErr, "All retry attempts failed",
		"method", req.Method,
		"endpoint", req.URL.String(),
		"max_attempts", h.config.MaxRetries,
	)
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
	log := log.FromContext(ctx).WithName("health-checker")

	// Log health check start
	log.V(1).Info("Starting health check",
		"endpoint", endpoint,
		"expected_ranges", expectedStatusCodeRanges,
	)

	// Perform the health check
	resp, err := h.performHealthCheck(ctx, endpoint, log)
	if err != nil {
		return false, 0, false, err
	}
	defer func() {
		if resp != nil {
			if err := resp.Body.Close(); err != nil {
				log.Error(err, "Failed to close response body")
			}
		}
	}()

	// Determine if the endpoint is healthy
	healthy := h.isStatusCodeHealthy(resp.StatusCode, expectedStatusCodeRanges)

	// Log health check result
	log.Info("Health check completed",
		"endpoint", endpoint,
		"status_code", resp.StatusCode,
		"healthy", healthy,
		"expected_ranges", expectedStatusCodeRanges,
	)

	// Report health status
	reportSuccess := h.reportHealthStatus(ctx, healthy, endpointsSecret, log)

	return healthy, resp.StatusCode, reportSuccess, nil
}

// performHealthCheck creates and executes the HTTP request for health checking.
func (h *DefaultHealthChecker) performHealthCheck(
	ctx context.Context,
	endpoint string,
	log logr.Logger,
) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		log.Error(err, "Health check failed",
			"endpoint", endpoint,
			"status_code", 0,
		)
		return nil, fmt.Errorf("%s: %w", ErrFailedToCreateRequest, err)
	}

	// Log request creation
	log.V(2).Info("HTTP request created",
		"method", req.Method,
		"endpoint", req.URL.String(),
		"user_agent", req.UserAgent(),
	)

	resp, err := h.doRequestWithRetries(req, log)
	if err != nil {
		log.Error(err, "Health check failed",
			"endpoint", endpoint,
			"status_code", 0,
		)
		return nil, err
	}

	return resp, nil
}

// isStatusCodeHealthy checks if the given status code falls within any of the expected ranges.
func (h *DefaultHealthChecker) isStatusCodeHealthy(
	statusCode int,
	expectedStatusCodeRanges []monitoringv1alpha1.StatusCodeRange,
) bool {
	for _, r := range expectedStatusCodeRanges {
		if statusCode >= r.Min && statusCode <= r.Max {
			return true
		}
	}
	return false
}

// reportHealthStatus reports the health status to the appropriate endpoint.
func (h *DefaultHealthChecker) reportHealthStatus(
	ctx context.Context,
	healthy bool,
	endpointsSecret monitoringv1alpha1.EndpointsSecret,
	log logr.Logger,
) bool {
	reportURL, reportMethod := h.getReportEndpoint(healthy, endpointsSecret)
	healthStatus := func() string {
		if healthy {
			return "healthy"
		}
		return "unhealthy"
	}()

	if reportURL == "" {
		log.V(2).Info("No report endpoint configured",
			"health_status", healthStatus,
		)
		return false
	}

	// Default to GET if method is not specified
	if reportMethod == "" {
		reportMethod = "GET"
	}

	// Log report attempt
	log.V(1).Info("Attempting to report health status",
		"health_status", healthStatus,
		"report_url", reportURL,
		"report_method", reportMethod,
	)

	return h.executeReportRequest(ctx, reportURL, reportMethod, healthy, log)
}

// getReportEndpoint returns the appropriate report URL and method based on health status.
func (h *DefaultHealthChecker) getReportEndpoint(
	healthy bool,
	endpointsSecret monitoringv1alpha1.EndpointsSecret,
) (string, string) {
	if healthy {
		return endpointsSecret.HealthyEndpointKey, endpointsSecret.HealthyEndpointMethod
	}
	return endpointsSecret.UnhealthyEndpointKey, endpointsSecret.UnhealthyEndpointMethod
}

// executeReportRequest performs the actual report request and handles the response.
func (h *DefaultHealthChecker) executeReportRequest(
	ctx context.Context,
	reportURL, reportMethod string,
	healthy bool,
	log logr.Logger,
) bool {
	healthStatus := h.getHealthStatusString(healthy)

	reportReq, err := h.createReportRequest(ctx, reportURL, reportMethod, healthStatus, log)
	if err != nil {
		return false
	}

	reportResp, err := h.doRequestWithRetries(reportReq, log)
	if err != nil {
		h.logReportRequestError(err, reportURL, reportMethod, healthStatus, log)
		return false
	}

	return h.handleReportResponse(reportResp, reportURL, reportMethod, healthStatus, log)
}

// getHealthStatusString returns a string representation of the health status.
func (h *DefaultHealthChecker) getHealthStatusString(healthy bool) string {
	if healthy {
		return "healthy"
	}
	return "unhealthy"
}

// createReportRequest creates the HTTP request for reporting health status.
func (h *DefaultHealthChecker) createReportRequest(
	ctx context.Context,
	reportURL, reportMethod, healthStatus string,
	log logr.Logger,
) (*http.Request, error) {
	reportReq, err := http.NewRequestWithContext(ctx, reportMethod, reportURL, nil)
	if err != nil {
		log.Error(err, "Report request failed",
			"report_url", reportURL,
			"report_method", reportMethod,
			"health_status", healthStatus,
		)
		return nil, err
	}
	return reportReq, nil
}

// logReportRequestError logs errors during report request execution.
func (h *DefaultHealthChecker) logReportRequestError(
	err error,
	reportURL, reportMethod, healthStatus string,
	log logr.Logger,
) {
	log.Error(err, "Report request failed",
		"report_url", reportURL,
		"report_method", reportMethod,
		"health_status", healthStatus,
	)
}

// handleReportResponse processes the report response and determines success.
func (h *DefaultHealthChecker) handleReportResponse(
	reportResp *http.Response,
	reportURL, reportMethod, healthStatus string,
	log logr.Logger,
) bool {
	if reportResp != nil && reportResp.StatusCode >= 200 && reportResp.StatusCode < 300 {
		log.V(1).Info("Report request successful",
			"report_url", reportURL,
			"report_method", reportMethod,
			"report_status_code", reportResp.StatusCode,
			"health_status", healthStatus,
		)
		return true
	}

	log.V(0).Info("Report request returned non-success status",
		"report_url", reportURL,
		"report_method", reportMethod,
		"report_status_code", reportResp.StatusCode,
		"health_status", healthStatus,
	)
	return false
}
