/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	monitoringv1alpha1 "github.com/siutsin/heartbeats/api/v1alpha1"
	"github.com/siutsin/heartbeats/internal/controller"
	"github.com/siutsin/heartbeats/test/mocks"
)

const (
	testNamespace = "default"
	testName      = "test-heartbeat"
	secretName    = "test-secret"
	interval      = "5s"
)

// testCase defines the structure for table-driven tests in the heartbeat controller.
// Each test case specifies the expected behaviour and provides setup functions.
type testCase struct {
	name           string                                              // Name of the test case
	statusCode     int                                                 // Expected HTTP status code
	expectedStatus int                                                 // Expected status code in the heartbeat status
	expectedMsg    string                                              // Expected status message
	expectHealthy  bool                                                // Whether the endpoint should be marked as healthy
	setupMock      func(*monitoringv1alpha1.Heartbeat, *corev1.Secret) // Function to set up test data
}

// TestHeartbeatReconciler runs a comprehensive test suite for the HeartbeatReconciler.
// It uses table-driven tests to verify various scenarios including healthy endpoints,
// unhealthy endpoints, invalid configurations, and error conditions.
func TestHeartbeatReconciler(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = monitoringv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tests := []testCase{
		{
			name:           "healthy endpoint",
			statusCode:     http.StatusOK,
			expectedStatus: http.StatusOK,
			expectedMsg:    controller.ErrEndpointHealthy,
			expectHealthy:  true,
			setupMock: func(h *monitoringv1alpha1.Heartbeat, s *corev1.Secret) {
				h.Spec.ExpectedStatusCodeRanges = []monitoringv1alpha1.StatusCodeRange{
					{Min: 200, Max: 299},
				}
				s.Data = map[string][]byte{
					"targetEndpoint":    []byte("https://example.com"),
					"healthyEndpoint":   []byte("https://healthy.example.com"),
					"unhealthyEndpoint": []byte("https://unhealthy.example.com"),
				}
			},
		},
		{
			name:           "unhealthy endpoint",
			statusCode:     http.StatusInternalServerError,
			expectedStatus: http.StatusInternalServerError,
			expectedMsg:    controller.ErrStatusCodeNotInRange,
			expectHealthy:  false,
			setupMock: func(h *monitoringv1alpha1.Heartbeat, s *corev1.Secret) {
				h.Spec.ExpectedStatusCodeRanges = []monitoringv1alpha1.StatusCodeRange{
					{Min: 200, Max: 299},
				}
				s.Data = map[string][]byte{
					"targetEndpoint":    []byte("https://example.com"),
					"healthyEndpoint":   []byte("https://healthy.example.com"),
					"unhealthyEndpoint": []byte("https://unhealthy.example.com"),
				}
			},
		},
		{
			name:           "invalid status code range",
			statusCode:     0, // No status code since health check should not be performed
			expectedStatus: 0,
			expectedMsg:    controller.ErrInvalidStatusCodeRange,
			expectHealthy:  false,
			setupMock: func(h *monitoringv1alpha1.Heartbeat, s *corev1.Secret) {
				h.Spec.ExpectedStatusCodeRanges = []monitoringv1alpha1.StatusCodeRange{
					{Min: 300, Max: 200},
				}
				s.Data = map[string][]byte{
					"targetEndpoint":    []byte("https://example.com"),
					"healthyEndpoint":   []byte("https://healthy.example.com"),
					"unhealthyEndpoint": []byte("https://unhealthy.example.com"),
				}
			},
		},
		{
			name:           "missing secret key",
			statusCode:     0,
			expectedStatus: 0,
			expectedMsg:    controller.ErrMissingRequiredKey,
			expectHealthy:  false,
			setupMock: func(h *monitoringv1alpha1.Heartbeat, s *corev1.Secret) {
				h.Spec.ExpectedStatusCodeRanges = []monitoringv1alpha1.StatusCodeRange{
					{Min: 200, Max: 299},
				}
				s.Data = map[string][]byte{}
			},
		},
		{
			name:           "empty endpoint",
			statusCode:     0,
			expectedStatus: 0,
			expectedMsg:    controller.ErrEndpointNotSpecified,
			expectHealthy:  false,
			setupMock: func(h *monitoringv1alpha1.Heartbeat, s *corev1.Secret) {
				h.Spec.ExpectedStatusCodeRanges = []monitoringv1alpha1.StatusCodeRange{
					{Min: 200, Max: 299},
				}
				s.Data = map[string][]byte{
					"targetEndpoint":    []byte(""),
					"healthyEndpoint":   []byte("https://healthy.example.com"),
					"unhealthyEndpoint": []byte("https://unhealthy.example.com"),
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			// Create test objects
			heartbeat := &monitoringv1alpha1.Heartbeat{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
				},
				Spec: monitoringv1alpha1.HeartbeatSpec{
					EndpointsSecret: monitoringv1alpha1.EndpointsSecret{
						Name:                 secretName,
						TargetEndpointKey:    "targetEndpoint",
						HealthyEndpointKey:   "healthyEndpoint",
						UnhealthyEndpointKey: "unhealthyEndpoint",
					},
					Interval: interval,
				},
			}

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
			}

			// Setup mock data
			tt.setupMock(heartbeat, secret)

			// Create fake client
			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(heartbeat, secret).
				WithStatusSubresource(heartbeat).
				Build()

			// Create generated mock for HealthChecker
			mockChecker := &mocks.HealthChecker{}
			mockChecker.On("CheckEndpointHealth", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(tt.expectHealthy, tt.statusCode, true, nil)

			// Create reconciler with generated mock
			reconciler := &controller.HeartbeatReconciler{
				Client:        client,
				Scheme:        scheme,
				Config:        controller.DefaultConfig(),
				HealthChecker: mockChecker,
				StatusUpdater: controller.NewStatusUpdater(client),
			}

			// Reconcile
			_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			})
			g.Expect(err).NotTo(gomega.HaveOccurred())

			// Verify status
			err = client.Get(context.Background(), types.NamespacedName{
				Name:      testName,
				Namespace: testNamespace,
			}, heartbeat)
			g.Expect(err).NotTo(gomega.HaveOccurred())
			g.Expect(heartbeat.Status.LastStatus).To(gomega.Equal(tt.expectedStatus))
			g.Expect(heartbeat.Status.Message).To(gomega.Equal(tt.expectedMsg))
			g.Expect(heartbeat.Status.Healthy).To(gomega.Equal(tt.expectHealthy))
			g.Expect(heartbeat.Status.LastChecked).NotTo(gomega.BeNil())
		})
	}
}

// TestConcurrentReconciliationNotBlocked verifies that a failing health check
// does not block other Heartbeat resources from being reconciled.
// This test simulates multiple Heartbeats where one has a failing endpoint that takes
// time to fail (simulating network timeout/retries), and verifies that the healthy
// Heartbeat completes its reconciliation without waiting for the failing one.
func TestConcurrentReconciliationNotBlocked(t *testing.T) {
	g := gomega.NewWithT(t)

	scheme := runtime.NewScheme()
	_ = monitoringv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create multiple heartbeats and secrets
	failingHeartbeat := &monitoringv1alpha1.Heartbeat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failing-heartbeat",
			Namespace: testNamespace,
		},
		Spec: monitoringv1alpha1.HeartbeatSpec{
			EndpointsSecret: monitoringv1alpha1.EndpointsSecret{
				Name:                 "failing-secret",
				TargetEndpointKey:    "targetEndpoint",
				HealthyEndpointKey:   "healthyEndpoint",
				UnhealthyEndpointKey: "unhealthyEndpoint",
			},
			ExpectedStatusCodeRanges: []monitoringv1alpha1.StatusCodeRange{{Min: 200, Max: 299}},
			Interval:                 interval,
		},
	}
	failingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failing-secret",
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"targetEndpoint":    []byte("https://unreachable.example.com"),
			"healthyEndpoint":   []byte("https://healthy.example.com"),
			"unhealthyEndpoint": []byte("https://unhealthy.example.com"),
		},
	}

	healthyHeartbeat := &monitoringv1alpha1.Heartbeat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "healthy-heartbeat",
			Namespace: testNamespace,
		},
		Spec: monitoringv1alpha1.HeartbeatSpec{
			EndpointsSecret: monitoringv1alpha1.EndpointsSecret{
				Name:                 "healthy-secret",
				TargetEndpointKey:    "targetEndpoint",
				HealthyEndpointKey:   "healthyEndpoint",
				UnhealthyEndpointKey: "unhealthyEndpoint",
			},
			ExpectedStatusCodeRanges: []monitoringv1alpha1.StatusCodeRange{{Min: 200, Max: 299}},
			Interval:                 interval,
		},
	}
	healthySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "healthy-secret",
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"targetEndpoint":    []byte("https://healthy.example.com"),
			"healthyEndpoint":   []byte("https://healthy.example.com"),
			"unhealthyEndpoint": []byte("https://unhealthy.example.com"),
		},
	}

	// Create fake client with both heartbeats
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(failingHeartbeat, failingSecret, healthyHeartbeat, healthySecret).
		WithStatusSubresource(failingHeartbeat, healthyHeartbeat).
		Build()

	// Simulate a failing endpoint that takes time to fail (like network timeout with retries)
	failureDelay := 500 * time.Millisecond
	networkErr := fmt.Errorf("dial tcp: lookup unreachable.example.com: no such host")

	mockChecker := &mocks.HealthChecker{}
	// Failing endpoint: delays then returns error (simulating timeout/retry behaviour)
	mockChecker.On("CheckEndpointHealth", mock.Anything, "https://unreachable.example.com", mock.Anything, mock.Anything).
		Run(func(_ mock.Arguments) {
			time.Sleep(failureDelay)
		}).
		Return(false, 0, false, networkErr)
	// Healthy endpoint: returns immediately
	mockChecker.On("CheckEndpointHealth", mock.Anything, "https://healthy.example.com", mock.Anything, mock.Anything).
		Return(true, http.StatusOK, true, nil)

	// Create reconciler
	reconciler := &controller.HeartbeatReconciler{
		Client:        fakeClient,
		Scheme:        scheme,
		Config:        controller.DefaultConfig(),
		HealthChecker: mockChecker,
		StatusUpdater: controller.NewStatusUpdater(fakeClient),
	}

	// Track completion times
	var failingCompleted, healthyCompleted time.Time
	startTime := time.Now()

	// Run reconciliations concurrently (simulating what the controller does with MaxConcurrentReconciles > 1)
	done := make(chan struct{})
	go func() {
		_, _ = reconciler.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "failing-heartbeat", Namespace: testNamespace},
		})
		failingCompleted = time.Now()
		done <- struct{}{}
	}()

	go func() {
		_, _ = reconciler.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "healthy-heartbeat", Namespace: testNamespace},
		})
		healthyCompleted = time.Now()
		done <- struct{}{}
	}()

	// Wait for both to complete
	<-done
	<-done

	// Verify healthy heartbeat completed significantly before failing heartbeat
	healthyDuration := healthyCompleted.Sub(startTime)
	failingDuration := failingCompleted.Sub(startTime)

	// Healthy should complete much quicker than the failure delay
	g.Expect(healthyDuration).To(gomega.BeNumerically("<", failureDelay),
		"healthy heartbeat should complete before failing endpoint times out")

	// Failing should take at least the delay time
	g.Expect(failingDuration).To(gomega.BeNumerically(">=", failureDelay),
		"failing heartbeat should take at least the failure delay time")

	// Verify healthy heartbeat was updated successfully
	err := fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "healthy-heartbeat", Namespace: testNamespace,
	}, healthyHeartbeat)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(healthyHeartbeat.Status.Healthy).To(gomega.BeTrue(),
		"healthy heartbeat should be marked as healthy")
	g.Expect(healthyHeartbeat.Status.Message).To(gomega.Equal(controller.ErrEndpointHealthy))

	// Verify failing heartbeat was updated with error status
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "failing-heartbeat", Namespace: testNamespace,
	}, failingHeartbeat)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(failingHeartbeat.Status.Healthy).To(gomega.BeFalse(),
		"failing heartbeat should be marked as unhealthy")
}

// TestParseInterval tests the parseInterval function with various valid and invalid interval strings.
func TestParseInterval(t *testing.T) {
	tests := []struct {
		name        string
		interval    string
		expectError bool
		expected    time.Duration
	}{
		{
			name:        "valid seconds",
			interval:    "30s",
			expectError: false,
			expected:    30 * time.Second,
		},
		{
			name:        "valid minutes",
			interval:    "5m",
			expectError: false,
			expected:    5 * time.Minute,
		},
		{
			name:        "valid hours",
			interval:    "1h",
			expectError: false,
			expected:    1 * time.Hour,
		},
		{
			name:        "valid mixed duration",
			interval:    "1h30m",
			expectError: false,
			expected:    1*time.Hour + 30*time.Minute,
		},
		{
			name:        "invalid format - no unit",
			interval:    "30",
			expectError: true,
		},
		{
			name:        "invalid format - invalid unit",
			interval:    "30x",
			expectError: true,
		},
		{
			name:        "negative duration (accepted by time.ParseDuration)",
			interval:    "-30s",
			expectError: false,
			expected:    -30 * time.Second,
		},
		{
			name:        "empty string",
			interval:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			result, err := controller.ParseInterval(tt.interval)

			if tt.expectError {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(result).To(gomega.Equal(tt.expected))
			}
		})
	}
}
