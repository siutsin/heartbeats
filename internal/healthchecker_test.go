package heartbeats_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/onsi/gomega"

	monitoringv1alpha1 "github.com/siutsin/heartbeats/api/v1alpha1"
	heartbeats "github.com/siutsin/heartbeats/internal"
)

const (
	testTimeout     = 1 * time.Second
	testRetryDelay  = 100 * time.Millisecond
	testMaxRetries  = 2
	testRequeueTime = 3 * time.Second
)

// Test assertion messages used throughout the test suite for consistent error reporting.
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

// TestNewHealthChecker verifies the factory returns a non-nil checker.
func TestNewHealthChecker(t *testing.T) {
	g := gomega.NewWithT(t)

	config := heartbeats.Config{
		DefaultTimeout: testTimeout,
		MaxRetries:     testMaxRetries,
		RetryDelay:     testRetryDelay,
		RequeueAfter:   testRequeueTime,
	}

	checker := heartbeats.NewHealthChecker(config)
	g.Expect(checker).NotTo(gomega.BeNil(), errNilChecker)
}

// newTestChecker returns a checker with fast timeouts for unit tests.
func newTestChecker() heartbeats.HealthChecker {
	return heartbeats.NewHealthChecker(heartbeats.Config{
		DefaultTimeout: testTimeout,
		MaxRetries:     testMaxRetries,
		RetryDelay:     testRetryDelay,
	})
}

// TestCheckEndpointHealth_HealthyEndpoint verifies in-range status codes report healthy.
func TestCheckEndpointHealth_HealthyEndpoint(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		expectedRange  []monitoringv1alpha1.StatusCodeRange
		serverBehavior func(http.ResponseWriter, *http.Request)
	}{
		{
			name:       "healthy endpoint within range",
			statusCode: http.StatusOK,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 200, Max: 299},
			},
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
		},
		{
			name:       "multiple status code ranges",
			statusCode: http.StatusAccepted,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 200, Max: 204},
				{Min: 300, Max: 399},
			},
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			},
		},
		{
			name:       "overlapping ranges",
			statusCode: http.StatusOK,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 200, Max: 299},
				{Min: 250, Max: 350},
			},
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
		},
		{
			name:       "boundary test - min",
			statusCode: http.StatusOK,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 200, Max: 200},
			},
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
		},
		{
			name:       "boundary test - max",
			statusCode: http.StatusMultipleChoices,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 300, Max: 300},
			},
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusMultipleChoices)
			},
		},
		{
			name:       "response with body",
			statusCode: http.StatusOK,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 200, Max: 299},
			},
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				if _, err := w.Write([]byte(`{"healthy":true,"lastStatus":200,"message":"Endpoint is healthy"}`)); err != nil {
					t.Errorf("failed to write response body: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			server := httptest.NewServer(http.HandlerFunc(tt.serverBehavior))
			defer server.Close()

			checker := newTestChecker()

			healthy, statusCode, _, err := checker.CheckEndpointHealth(
				context.Background(),
				server.URL,
				tt.expectedRange,
				monitoringv1alpha1.EndpointsSecret{}, // dummy for legacy tests
			)

			g.Expect(err).NotTo(gomega.HaveOccurred(), errUnexpectedError)
			g.Expect(healthy).To(gomega.BeTrue(), errHealthyMismatch)
			g.Expect(statusCode).To(gomega.Equal(tt.statusCode), errStatusCodeMismatch)
		})
	}
}

// TestCheckEndpointHealth_UnhealthyEndpoint verifies out-of-range codes report unhealthy.
func TestCheckEndpointHealth_UnhealthyEndpoint(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		expectedRange  []monitoringv1alpha1.StatusCodeRange
		serverBehavior func(http.ResponseWriter, *http.Request)
	}{
		{
			name:       "unhealthy endpoint outside range",
			statusCode: http.StatusInternalServerError,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{
				{Min: 200, Max: 299},
			},
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
		},
		{
			name:          "empty status code ranges",
			statusCode:    http.StatusOK,
			expectedRange: []monitoringv1alpha1.StatusCodeRange{},
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			server := httptest.NewServer(http.HandlerFunc(tt.serverBehavior))
			defer server.Close()

			checker := newTestChecker()

			healthy, statusCode, _, err := checker.CheckEndpointHealth(
				context.Background(),
				server.URL,
				tt.expectedRange,
				monitoringv1alpha1.EndpointsSecret{}, // dummy for legacy tests
			)

			g.Expect(err).NotTo(gomega.HaveOccurred(), errUnexpectedError)
			g.Expect(healthy).To(gomega.BeFalse(), errHealthyMismatch)
			g.Expect(statusCode).To(gomega.Equal(tt.statusCode), errStatusCodeMismatch)
		})
	}
}

// TestCheckEndpointHealth_NetworkErrors verifies network failures return errors.
func TestCheckEndpointHealth_NetworkErrors(t *testing.T) {
	tests := []struct {
		name           string
		endpoint       string
		expectedErrMsg string
		serverBehavior func(http.ResponseWriter, *http.Request)
	}{
		{
			name:           "invalid URL",
			endpoint:       "http://invalid-url:invalid-port",
			expectedErrMsg: heartbeats.ErrFailedToCreateRequest,
		},
		{
			name:           "connection refused",
			endpoint:       "http://localhost:9999", // Assuming this port is not in use
			expectedErrMsg: heartbeats.ErrFailedToMakeRequest,
		},
		{
			name:           "DNS error",
			endpoint:       "http://nonexistent-domain-that-does-not-exist.com",
			expectedErrMsg: heartbeats.ErrFailedToMakeRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			checker := newTestChecker()

			healthy, statusCode, _, err := checker.CheckEndpointHealth(
				context.Background(),
				tt.endpoint,
				[]monitoringv1alpha1.StatusCodeRange{{Min: 200, Max: 299}},
				monitoringv1alpha1.EndpointsSecret{}, // dummy for legacy tests
			)

			g.Expect(err).To(gomega.HaveOccurred(), errExpectedError)
			g.Expect(err.Error()).To(gomega.ContainSubstring(tt.expectedErrMsg), errMessageMismatch)
			g.Expect(healthy).To(gomega.BeFalse(), errHealthyMismatch)
			g.Expect(statusCode).To(gomega.Equal(0), errStatusCodeMismatch)
		})
	}
}

// TestCheckEndpointHealth_TimeoutErrors verifies timeouts return errors.
func TestCheckEndpointHealth_TimeoutErrors(t *testing.T) {
	tests := []struct {
		name           string
		expectedErrMsg string
		serverBehavior func(http.ResponseWriter, *http.Request)
		setupContext   func() context.Context
	}{
		{
			name:           "server timeout",
			expectedErrMsg: heartbeats.ErrEndpointTimeout,
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(2 * time.Second)
				w.WriteHeader(http.StatusOK)
			},
			setupContext: func() context.Context { return context.Background() },
		},
		{
			name:           "context cancelled",
			expectedErrMsg: "context canceled",
			serverBehavior: func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(500 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
			},
			setupContext: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
				return ctx
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			server := httptest.NewServer(http.HandlerFunc(tt.serverBehavior))
			defer server.Close()

			checker := newTestChecker()

			ctx := tt.setupContext()
			healthy, statusCode, _, err := checker.CheckEndpointHealth(
				ctx,
				server.URL,
				[]monitoringv1alpha1.StatusCodeRange{{Min: 200, Max: 299}},
				monitoringv1alpha1.EndpointsSecret{}, // dummy for legacy tests
			)

			g.Expect(err).To(gomega.HaveOccurred(), errExpectedError)
			g.Expect(err.Error()).To(gomega.ContainSubstring(tt.expectedErrMsg), errMessageMismatch)
			g.Expect(healthy).To(gomega.BeFalse(), errHealthyMismatch)
			g.Expect(statusCode).To(gomega.Equal(0), errStatusCodeMismatch)
		})
	}
}

// TestCheckEndpointHealth_ReportsToCorrectEndpoint verifies reports reach the healthy or unhealthy endpoint.
func TestCheckEndpointHealth_ReportsToCorrectEndpoint(t *testing.T) {
	g := gomega.NewWithT(t)

	// Set up a server to act as the main endpoint
	mainStatus := http.StatusOK
	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(mainStatus)
	}))
	defer mainServer.Close()

	// Set up servers to act as the healthy and unhealthy report endpoints
	reportCalled := ""
	healthyReportServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reportCalled = "healthy"
		w.WriteHeader(http.StatusNoContent)
	}))
	defer healthyReportServer.Close()

	unhealthyReportServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reportCalled = "unhealthy"
		w.WriteHeader(http.StatusNoContent)
	}))
	defer unhealthyReportServer.Close()

	checker := newTestChecker()

	ctx := context.Background()
	statusRanges := []monitoringv1alpha1.StatusCodeRange{{Min: 200, Max: 299}}
	endpointsSecret := monitoringv1alpha1.EndpointsSecret{
		HealthyEndpointKey:   healthyReportServer.URL,
		UnhealthyEndpointKey: unhealthyReportServer.URL,
	}

	// Test healthy case
	reportCalled = ""
	mainStatus = http.StatusOK
	healthy, _, _, err := checker.CheckEndpointHealth(ctx, mainServer.URL, statusRanges, endpointsSecret)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(healthy).To(gomega.BeTrue())
	g.Eventually(func() string { return reportCalled }).Should(gomega.Equal("healthy"))

	// Test unhealthy case
	reportCalled = ""
	mainStatus = http.StatusInternalServerError
	healthy, _, _, err = checker.CheckEndpointHealth(ctx, mainServer.URL, statusRanges, endpointsSecret)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(healthy).To(gomega.BeFalse())
	g.Eventually(func() string { return reportCalled }).Should(gomega.Equal("unhealthy"))
}
