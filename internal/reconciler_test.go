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

package heartbeats_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	monitoringv1alpha1 "github.com/siutsin/heartbeats/api/v1alpha1"
	heartbeats "github.com/siutsin/heartbeats/internal"
)

const (
	testNamespace = "default"
	testName      = "test-heartbeat"
	secretName    = "test-secret"
	interval      = "5s"
)

// testCase is one row of the Reconciler table test.
type testCase struct {
	name           string
	statusCode     int
	expectedStatus int
	expectedMsg    string
	expectHealthy  bool
	setup          func(*monitoringv1alpha1.Heartbeat, *corev1.Secret)
}

// TestReconciler verifies reconcile outcomes across endpoint configurations.
func TestReconciler(t *testing.T) {
	g := gomega.NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(monitoringv1alpha1.AddToScheme(scheme)).To(gomega.Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(gomega.Succeed())

	tests := []testCase{
		{
			name:           "healthy endpoint",
			statusCode:     http.StatusOK,
			expectedStatus: http.StatusOK,
			expectedMsg:    heartbeats.ErrEndpointHealthy,
			expectHealthy:  true,
			setup: func(h *monitoringv1alpha1.Heartbeat, s *corev1.Secret) {
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
			expectedMsg:    heartbeats.ErrStatusCodeNotInRange,
			expectHealthy:  false,
			setup: func(h *monitoringv1alpha1.Heartbeat, s *corev1.Secret) {
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
			expectedMsg:    heartbeats.ErrInvalidStatusCodeRange,
			expectHealthy:  false,
			setup: func(h *monitoringv1alpha1.Heartbeat, s *corev1.Secret) {
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
			expectedMsg:    heartbeats.ErrMissingRequiredKey,
			expectHealthy:  false,
			setup: func(h *monitoringv1alpha1.Heartbeat, s *corev1.Secret) {
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
			expectedMsg:    heartbeats.ErrEndpointNotSpecified,
			expectHealthy:  false,
			setup: func(h *monitoringv1alpha1.Heartbeat, s *corev1.Secret) {
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

			tt.setup(heartbeat, secret)

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(heartbeat, secret).
				WithStatusSubresource(heartbeat).
				Build()

			checker := &fakeHealthChecker{
				check: func(context.Context, string, []monitoringv1alpha1.StatusCodeRange, monitoringv1alpha1.EndpointsSecret) (bool, int, bool, error) {
					return tt.expectHealthy, tt.statusCode, true, nil
				},
			}

			reconciler := &heartbeats.Reconciler{
				Client:        client,
				Config:        heartbeats.DefaultConfig(),
				HealthChecker: checker,
				StatusUpdater: heartbeats.NewStatusUpdater(client),
			}

			_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			})
			g.Expect(err).NotTo(gomega.HaveOccurred())

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

// TestConcurrentReconciliationNotBlocked verifies a slow failing check does not block a healthy one.
func TestConcurrentReconciliationNotBlocked(t *testing.T) {
	g := gomega.NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(monitoringv1alpha1.AddToScheme(scheme)).To(gomega.Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(gomega.Succeed())

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

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(failingHeartbeat, failingSecret, healthyHeartbeat, healthySecret).
		WithStatusSubresource(failingHeartbeat, healthyHeartbeat).
		Build()

	failureDelay := 500 * time.Millisecond
	networkErr := fmt.Errorf("dial tcp: lookup unreachable.example.com: no such host")

	checker := &fakeHealthChecker{
		check: func(_ context.Context, endpoint string, _ []monitoringv1alpha1.StatusCodeRange, _ monitoringv1alpha1.EndpointsSecret) (bool, int, bool, error) {
			// Failing endpoint: delays then returns error (simulating timeout/retry behaviour).
			// Healthy endpoint: returns immediately.
			if endpoint == "https://unreachable.example.com" {
				time.Sleep(failureDelay)
				return false, 0, false, networkErr
			}
			return true, http.StatusOK, true, nil
		},
	}

	reconciler := &heartbeats.Reconciler{
		Client:        fakeClient,
		Config:        heartbeats.DefaultConfig(),
		HealthChecker: checker,
		StatusUpdater: heartbeats.NewStatusUpdater(fakeClient),
	}

	var failingCompleted, healthyCompleted time.Time
	startTime := time.Now()

	errCh := make(chan error, 2)
	go func() {
		_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "failing-heartbeat", Namespace: testNamespace},
		})
		failingCompleted = time.Now()
		errCh <- err
	}()

	go func() {
		_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "healthy-heartbeat", Namespace: testNamespace},
		})
		healthyCompleted = time.Now()
		errCh <- err
	}()

	g.Expect(<-errCh).NotTo(gomega.HaveOccurred())
	g.Expect(<-errCh).NotTo(gomega.HaveOccurred())

	healthyDuration := healthyCompleted.Sub(startTime)
	failingDuration := failingCompleted.Sub(startTime)

	g.Expect(healthyDuration).To(gomega.BeNumerically("<", failureDelay),
		"healthy heartbeat should complete before failing endpoint times out")

	g.Expect(failingDuration).To(gomega.BeNumerically(">=", failureDelay),
		"failing heartbeat should take at least the failure delay time")

	err := fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "healthy-heartbeat", Namespace: testNamespace,
	}, healthyHeartbeat)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(healthyHeartbeat.Status.Healthy).To(gomega.BeTrue(),
		"healthy heartbeat should be marked as healthy")
	g.Expect(healthyHeartbeat.Status.Message).To(gomega.Equal(heartbeats.ErrEndpointHealthy))

	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "failing-heartbeat", Namespace: testNamespace,
	}, failingHeartbeat)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(failingHeartbeat.Status.Healthy).To(gomega.BeFalse(),
		"failing heartbeat should be marked as unhealthy")
}

// TestParseInterval verifies duration parsing, including invalid input.
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

			result, err := heartbeats.ParseInterval(tt.interval)

			if tt.expectError {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(result).To(gomega.Equal(tt.expected))
			}
		})
	}
}
