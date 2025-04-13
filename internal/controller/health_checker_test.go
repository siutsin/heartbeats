package controller_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/onsi/gomega"

	monitoringv1alpha1 "github.com/siutsin/heartbeats/api/v1alpha1"
	"github.com/siutsin/heartbeats/internal/controller"
)

const (
	testTimeout     = 1 * time.Second
	testRetryDelay  = 100 * time.Millisecond
	testMaxRetries  = 2
	testRequeueTime = 3 * time.Second
)

// Test assertion messages
const (
	errNilChecker          = "health checker should not be nil"
	errConfigMismatch      = "config should match"
	errUnexpectedError     = "unexpected error occurred"
	errExpectedError       = "expected error did not occur"
	errHealthyMismatch     = "healthy status mismatch"
	errStatusCodeMismatch  = "status code mismatch"
	errTypeAssertionFailed = "type assertion failed"
	errMessageMismatch     = "error message mismatch"
)

func TestNewHealthChecker(t *testing.T) {
	g := gomega.NewWithT(t)

	config := controller.Config{
		DefaultTimeout: testTimeout,
		MaxRetries:     testMaxRetries,
		RetryDelay:     testRetryDelay,
		RequeueAfter:   testRequeueTime,
	}

	checker := controller.NewHealthChecker(config)
	g.Expect(checker).NotTo(gomega.BeNil(), errNilChecker)
}

func TestCheckEndpointHealth(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		expectedRange  []monitoringv1alpha1.StatusCodeRange
		expectHealthy  bool
		expectError    bool
		expectedErrMsg string
		serverBehavior func(http.ResponseWriter, *http.Request)
	}{
		{
			name:          "healthy endpoint within range",
			statusCode:    http.StatusOK,
			expectHealthy: true,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 200, Max: 299},
			},
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
		},
		{
			name:          "unhealthy endpoint outside range",
			statusCode:    http.StatusInternalServerError,
			expectHealthy: false,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 200, Max: 299},
			},
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
		},
		{
			name:          "multiple status code ranges",
			statusCode:    http.StatusAccepted,
			expectHealthy: true,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 200, Max: 204},
				{Min: 300, Max: 399},
			},
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			},
		},
		{
			name:           "server timeout",
			expectHealthy:  false,
			expectError:    true,
			expectedErrMsg: controller.ErrEndpointTimeout,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 200, Max: 299},
			},
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(2 * time.Second)
				w.WriteHeader(http.StatusOK)
			},
		},
		{
			name:          "empty status code ranges",
			statusCode:    http.StatusOK,
			expectHealthy: false,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{},
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
		},
		{
			name:           "invalid URL",
			expectHealthy:  false,
			expectError:    true,
			expectedErrMsg: controller.ErrFailedToCreateRequest,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 200, Max: 299},
			},
			serverBehavior: nil, // No server needed for invalid URL test
		},
		{
			name:           "connection refused",
			expectHealthy:  false,
			expectError:    true,
			expectedErrMsg: controller.ErrFailedToMakeRequest,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 200, Max: 299},
			},
			serverBehavior: nil, // No server needed for connection refused test
		},
		{
			name:           "DNS error",
			expectHealthy:  false,
			expectError:    true,
			expectedErrMsg: controller.ErrFailedToMakeRequest,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 200, Max: 299},
			},
			serverBehavior: nil, // No server needed for DNS error test
		},
		{
			name:           "context cancelled",
			expectHealthy:  false,
			expectError:    true,
			expectedErrMsg: controller.ErrEndpointTimeout,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 200, Max: 299},
			},
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(500 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
			},
		},
		{
			name:          "overlapping ranges",
			statusCode:    http.StatusOK,
			expectHealthy: true,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 200, Max: 299},
				{Min: 250, Max: 350},
			},
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
		},
		{
			name:          "boundary test - min",
			statusCode:    http.StatusOK,
			expectHealthy: true,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 200, Max: 200},
			},
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
		},
		{
			name:          "boundary test - max",
			statusCode:    http.StatusMultipleChoices,
			expectHealthy: true,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 300, Max: 300},
			},
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusMultipleChoices)
			},
		},
		{
			name:          "response with body",
			statusCode:    http.StatusOK,
			expectHealthy: true,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 200, Max: 299},
			},
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"healthy":true,"lastStatus":200,"message":"Endpoint is healthy"}`))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			var server *httptest.Server
			var endpoint string

			if tt.serverBehavior != nil {
				server = httptest.NewServer(http.HandlerFunc(tt.serverBehavior))
				defer server.Close()
				endpoint = server.URL
			} else {
				switch tt.name {
				case "invalid URL":
					endpoint = "http://invalid-url:invalid-port"
				case "connection refused":
					endpoint = "http://localhost:9999" // Assuming this port is not in use
				case "DNS error":
					endpoint = "http://nonexistent-domain-that-does-not-exist.com"
				}
			}

			checker := controller.NewHealthChecker(controller.Config{
				DefaultTimeout: testTimeout,
				MaxRetries:     testMaxRetries,
				RetryDelay:     testRetryDelay,
			})

			ctx := context.Background()
			if tt.name == "context cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}

			healthy, statusCode, err := checker.CheckEndpointHealth(
				ctx,
				endpoint,
				tt.expectedRange,
			)

			if tt.expectError {
				g.Expect(err).To(gomega.HaveOccurred(), errExpectedError)
				if tt.expectedErrMsg != "" {
					g.Expect(err.Error()).To(gomega.ContainSubstring(tt.expectedErrMsg), errMessageMismatch)
				}
			} else {
				g.Expect(err).NotTo(gomega.HaveOccurred(), errUnexpectedError)
				g.Expect(healthy).To(gomega.Equal(tt.expectHealthy), errHealthyMismatch)
				g.Expect(statusCode).To(gomega.Equal(tt.statusCode), errStatusCodeMismatch)
			}
		})
	}
}
