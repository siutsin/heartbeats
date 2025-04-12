package controller

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1alpha1 "github.com/siutsin/heartbeats/api/v1alpha1"
)

// HealthChecker is an interface for checking endpoint health
type HealthChecker interface {
	CheckEndpointHealth(
		ctx context.Context,
		endpoint string,
		expectedStatusCodeRanges []monitoringv1alpha1.StatusCodeRange,
	) (bool, int, error)
}

// DefaultHealthChecker is the default implementation of HealthChecker
type DefaultHealthChecker struct {
	config Config
}

// NewHealthChecker creates a new DefaultHealthChecker
func NewHealthChecker(config Config) HealthChecker {
	return &DefaultHealthChecker{
		config: config,
	}
}

// CheckEndpointHealth checks if the endpoint is healthy by making an HTTP request
func (h *DefaultHealthChecker) CheckEndpointHealth(
	ctx context.Context,
	endpoint string,
	expectedStatusCodeRanges []monitoringv1alpha1.StatusCodeRange,
) (bool, int, error) {
	log := log.FromContext(ctx)

	client := &http.Client{
		Timeout: h.config.DefaultTimeout,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		log.Error(err, ErrFailedToCreateRequest, "endpoint", endpoint)
		return false, 0, fmt.Errorf("%s: %w", ErrFailedToCreateRequest, err)
	}

	var lastErr error
	for i := 0; i < h.config.MaxRetries; i++ {
		resp, err := client.Do(req)
		if err != nil {
			log.Error(err, "Failed to make request", "endpoint", endpoint, "attempt", i+1)
			if err, ok := err.(net.Error); ok && err.Timeout() {
				log.Error(err, ErrEndpointTimeout, "timeout", h.config.DefaultTimeout)
				lastErr = fmt.Errorf("%s: %w", ErrEndpointTimeout, err)
			} else {
				lastErr = fmt.Errorf("%s: %w", ErrFailedToMakeRequest, err)
			}
			time.Sleep(h.config.RetryDelay)
			continue
		}
		if resp != nil {
			defer func() {
				if err := resp.Body.Close(); err != nil {
					log.Error(err, "Failed to close response body")
				}
			}()
		}

		for _, r := range expectedStatusCodeRanges {
			if resp.StatusCode >= r.Min && resp.StatusCode <= r.Max {
				return true, resp.StatusCode, nil
			}
		}
		return false, resp.StatusCode, nil
	}

	if lastErr != nil {
		return false, 0, lastErr
	}

	log.Error(nil, ErrFailedToMakeRequest, "attempts", h.config.MaxRetries)
	return false, 0, errors.New(ErrFailedToMakeRequest)
}
