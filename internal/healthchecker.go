package heartbeats

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1alpha1 "github.com/siutsin/heartbeats/api/v1alpha1"
)

// HealthChecker is an interface for checking endpoint health.
type HealthChecker interface {
	CheckEndpointHealth(
		ctx context.Context,
		endpoint string,
		expectedStatusCodeRanges []monitoringv1alpha1.StatusCodeRange,
		endpointsSecret monitoringv1alpha1.EndpointsSecret,
	) (bool, int, bool, error)
}

// DefaultHealthChecker is the default implementation of HealthChecker.
type DefaultHealthChecker struct {
	config Config
	client *http.Client
}

// NewHealthChecker creates a new DefaultHealthChecker with the provided configuration.
func NewHealthChecker(config Config) HealthChecker {
	return &DefaultHealthChecker{
		config: config,
		client: &http.Client{Timeout: config.DefaultTimeout},
	}
}

func (h *DefaultHealthChecker) doRequestWithRetries(req *http.Request, log logr.Logger) (*http.Response, error) {
	var lastErr error
	for i := 0; i < h.config.MaxRetries; i++ {
		resp, err := h.client.Do(req)
		if err == nil {
			return resp, nil
		}

		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			lastErr = fmt.Errorf("%s: %w", ErrEndpointTimeout, err)
		} else {
			lastErr = fmt.Errorf("%s: %w", ErrFailedToMakeRequest, err)
		}
		log.Error(lastErr, "HTTP request failed",
			"method", req.Method,
			"endpoint", req.URL.String(),
			"attempt", i+1,
		)
		if i < h.config.MaxRetries-1 {
			time.Sleep(h.config.RetryDelay)
		}
	}
	return nil, lastErr
}

func (h *DefaultHealthChecker) CheckEndpointHealth(
	ctx context.Context,
	endpoint string,
	expectedStatusCodeRanges []monitoringv1alpha1.StatusCodeRange,
	endpointsSecret monitoringv1alpha1.EndpointsSecret,
) (bool, int, bool, error) {
	log := log.FromContext(ctx).WithName("health-checker")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, 0, false, fmt.Errorf("%s: %w", ErrFailedToCreateRequest, err)
	}

	resp, err := h.doRequestWithRetries(req, log)
	if err != nil {
		return false, 0, false, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Error(closeErr, "Failed to close response body")
		}
	}()

	healthy := h.isStatusCodeHealthy(resp.StatusCode, expectedStatusCodeRanges)
	reportSuccess := h.reportHealthStatus(ctx, healthy, endpointsSecret, log)
	return healthy, resp.StatusCode, reportSuccess, nil
}

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

func (h *DefaultHealthChecker) reportHealthStatus(
	ctx context.Context,
	healthy bool,
	endpointsSecret monitoringv1alpha1.EndpointsSecret,
	log logr.Logger,
) bool {
	reportURL, reportMethod := h.getReportEndpoint(healthy, endpointsSecret)
	if reportURL == "" {
		return false
	}
	if reportMethod == "" {
		reportMethod = "GET"
	}

	reportReq, err := http.NewRequestWithContext(ctx, reportMethod, reportURL, nil)
	if err != nil {
		log.Error(err, "Report request failed", "report_url", reportURL, "report_method", reportMethod)
		return false
	}

	reportResp, err := h.doRequestWithRetries(reportReq, log)
	if err != nil {
		log.Error(err, "Report request failed", "report_url", reportURL, "report_method", reportMethod)
		return false
	}
	defer func() {
		if closeErr := reportResp.Body.Close(); closeErr != nil {
			log.Error(closeErr, "Failed to close report response body")
		}
	}()

	return reportResp.StatusCode >= 200 && reportResp.StatusCode < 300
}

func (h *DefaultHealthChecker) getReportEndpoint(
	healthy bool,
	endpointsSecret monitoringv1alpha1.EndpointsSecret,
) (string, string) {
	if healthy {
		return endpointsSecret.HealthyEndpointKey, endpointsSecret.HealthyEndpointMethod
	}
	return endpointsSecret.UnhealthyEndpointKey, endpointsSecret.UnhealthyEndpointMethod
}
