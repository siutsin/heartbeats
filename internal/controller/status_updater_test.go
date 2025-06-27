package controller_test

import (
	"context"
	"errors"
	"testing"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	monitoringv1alpha1 "github.com/siutsin/heartbeats/api/v1alpha1"
	"github.com/siutsin/heartbeats/internal/controller"
)

// setupScheme creates and returns a runtime scheme with the monitoring API types registered.
// This is used by tests to ensure proper type handling when working with fake clients.
//
// Returns:
//   - *runtime.Scheme: A scheme with monitoring API types registered
func setupScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = monitoringv1alpha1.AddToScheme(scheme)
	return scheme
}

// createTestHeartbeat creates a test heartbeat with default metadata.
// This helper function reduces code duplication in tests.
//
// Returns:
//   - *monitoringv1alpha1.Heartbeat: A test heartbeat instance
func createTestHeartbeat() *monitoringv1alpha1.Heartbeat {
	return &monitoringv1alpha1.Heartbeat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-heartbeat",
			Namespace: "default",
		},
	}
}

// createTestClient creates a fake client with the test heartbeat.
// This helper function reduces code duplication in tests.
//
// Parameters:
//   - heartbeat: The heartbeat object to include in the fake client
//
// Returns:
//   - client.Client: A fake client with the heartbeat registered
func createTestClient(heartbeat *monitoringv1alpha1.Heartbeat) client.Client {
	return fake.NewClientBuilder().
		WithScheme(setupScheme()).
		WithObjects(heartbeat).
		WithStatusSubresource(heartbeat).
		Build()
}

// TestNewStatusUpdater verifies that a new StatusUpdater can be created with the provided client.
// This test ensures the factory function works correctly and returns a properly configured instance.
func TestNewStatusUpdater(t *testing.T) {
	g := gomega.NewWithT(t)

	client := fake.NewClientBuilder().WithScheme(setupScheme()).Build()
	updater := controller.NewStatusUpdater(client)

	g.Expect(updater).NotTo(gomega.BeNil())
	g.Expect(updater.Client).To(gomega.Equal(client))
}

// TestUpdateStatus_Basic tests the basic status update functionality.
// It verifies that the core UpdateStatus method correctly sets all status fields.
func TestUpdateStatus_Basic(t *testing.T) {
	g := gomega.NewWithT(t)

	heartbeat := &monitoringv1alpha1.Heartbeat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-heartbeat",
			Namespace: "default",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(setupScheme()).
		WithObjects(heartbeat).
		WithStatusSubresource(heartbeat).
		Build()

	updater := controller.NewStatusUpdater(client)

	err := updater.UpdateStatus(context.Background(), heartbeat, 200, true, "test message")

	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(heartbeat.Status.LastStatus).To(gomega.Equal(200))
	g.Expect(heartbeat.Status.Healthy).To(gomega.BeTrue())
	g.Expect(heartbeat.Status.Message).To(gomega.Equal("test message"))
	g.Expect(heartbeat.Status.LastChecked).NotTo(gomega.BeNil())
}

// TestUpdateStatus_ErrorConditions tests status updates for various error conditions.
// It verifies that different error scenarios are handled correctly with appropriate status messages.
func TestUpdateStatus_ErrorConditions(t *testing.T) {
	tests := []struct {
		name            string
		updateFunc      func(*controller.StatusUpdater, context.Context, *monitoringv1alpha1.Heartbeat) error
		expectedStatus  int
		expectedHealthy bool
		expectedMsg     string
	}{
		{
			name: "update secret error status",
			updateFunc: func(u *controller.StatusUpdater, ctx context.Context, h *monitoringv1alpha1.Heartbeat) error {
				return u.UpdateSecretErrorStatus(ctx, h, errors.New("test error"))
			},
			expectedStatus:  0,
			expectedHealthy: false,
			expectedMsg:     controller.ErrFailedToGetSecret,
		},
		{
			name: "update missing key status",
			updateFunc: func(u *controller.StatusUpdater, ctx context.Context, h *monitoringv1alpha1.Heartbeat) error {
				return u.UpdateMissingKeyStatus(ctx, h, "test-key")
			},
			expectedStatus:  0,
			expectedHealthy: false,
			expectedMsg:     controller.ErrMissingRequiredKey,
		},
		{
			name: "update empty endpoint status",
			updateFunc: func(u *controller.StatusUpdater, ctx context.Context, h *monitoringv1alpha1.Heartbeat) error {
				return u.UpdateEmptyEndpointStatus(ctx, h)
			},
			expectedStatus:  0,
			expectedHealthy: false,
			expectedMsg:     controller.ErrEndpointNotSpecified,
		},
		{
			name: "update health check error status",
			updateFunc: func(u *controller.StatusUpdater, ctx context.Context, h *monitoringv1alpha1.Heartbeat) error {
				return u.UpdateHealthCheckErrorStatus(ctx, h, 500, errors.New("test error"))
			},
			expectedStatus:  500,
			expectedHealthy: false,
			expectedMsg:     controller.ErrFailedToCheckEndpoint,
		},
		{
			name: "update invalid range status",
			updateFunc: func(u *controller.StatusUpdater, ctx context.Context, h *monitoringv1alpha1.Heartbeat) error {
				return u.UpdateInvalidRangeStatus(ctx, h, 500)
			},
			expectedStatus:  500,
			expectedHealthy: false,
			expectedMsg:     controller.ErrInvalidStatusCodeRange,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			heartbeat := &monitoringv1alpha1.Heartbeat{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-heartbeat",
					Namespace: "default",
				},
			}

			client := fake.NewClientBuilder().
				WithScheme(setupScheme()).
				WithObjects(heartbeat).
				WithStatusSubresource(heartbeat).
				Build()

			updater := controller.NewStatusUpdater(client)

			err := tt.updateFunc(updater, context.Background(), heartbeat)

			g.Expect(err).NotTo(gomega.HaveOccurred())
			g.Expect(heartbeat.Status.LastStatus).To(gomega.Equal(tt.expectedStatus))
			g.Expect(heartbeat.Status.Healthy).To(gomega.Equal(tt.expectedHealthy))
			g.Expect(heartbeat.Status.Message).To(gomega.Equal(tt.expectedMsg))
			g.Expect(heartbeat.Status.LastChecked).NotTo(gomega.BeNil())
		})
	}
}

// TestUpdateHealthStatus_HealthyEndpoint tests health status updates for healthy endpoints.
// It verifies that healthy endpoints are correctly marked with appropriate status messages.
func TestUpdateHealthStatus_HealthyEndpoint(t *testing.T) {
	g := gomega.NewWithT(t)

	heartbeat := createTestHeartbeat()
	client := createTestClient(heartbeat)
	updater := controller.NewStatusUpdater(client)

	err := updater.UpdateHealthStatus(
		context.Background(),
		heartbeat,
		true, // healthy
		200,  // status code
		nil,  // no error
		true, // report success
	)

	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(heartbeat.Status.LastStatus).To(gomega.Equal(200))
	g.Expect(heartbeat.Status.Healthy).To(gomega.BeTrue())
	g.Expect(heartbeat.Status.Message).To(gomega.Equal(controller.ErrEndpointHealthy))
	g.Expect(heartbeat.Status.ReportStatus).To(gomega.Equal("Success"))
}

// TestUpdateHealthStatus_UnhealthyEndpoint tests health status updates for unhealthy endpoints.
// It verifies that unhealthy endpoints are correctly marked with appropriate status messages.
func TestUpdateHealthStatus_UnhealthyEndpoint(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		err         error
		expectedMsg string
	}{
		{
			name:        "status code not in range",
			statusCode:  500,
			err:         nil,
			expectedMsg: controller.ErrStatusCodeNotInRange,
		},
		{
			name:        "error checking health",
			statusCode:  0,
			err:         errors.New("connection refused"),
			expectedMsg: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			heartbeat := createTestHeartbeat()
			client := createTestClient(heartbeat)
			updater := controller.NewStatusUpdater(client)

			err := updater.UpdateHealthStatus(
				context.Background(),
				heartbeat,
				false, // unhealthy
				tt.statusCode,
				tt.err,
				true, // report success
			)

			g.Expect(err).NotTo(gomega.HaveOccurred())
			g.Expect(heartbeat.Status.LastStatus).To(gomega.Equal(tt.statusCode))
			g.Expect(heartbeat.Status.Healthy).To(gomega.BeFalse())
			g.Expect(heartbeat.Status.Message).To(gomega.Equal(tt.expectedMsg))
			g.Expect(heartbeat.Status.ReportStatus).To(gomega.Equal("Success"))
		})
	}
}

// TestUpdateHealthStatus_ReportFailure tests health status updates when reporting fails.
// It verifies that report failures are correctly reflected in the status.
func TestUpdateHealthStatus_ReportFailure(t *testing.T) {
	g := gomega.NewWithT(t)

	heartbeat := createTestHeartbeat()
	client := createTestClient(heartbeat)
	updater := controller.NewStatusUpdater(client)

	err := updater.UpdateHealthStatus(
		context.Background(),
		heartbeat,
		true,  // healthy
		200,   // status code
		nil,   // no error
		false, // report failure
	)

	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(heartbeat.Status.LastStatus).To(gomega.Equal(200))
	g.Expect(heartbeat.Status.Healthy).To(gomega.BeTrue())
	g.Expect(heartbeat.Status.Message).To(gomega.Equal(controller.ErrEndpointHealthy))
	g.Expect(heartbeat.Status.ReportStatus).To(gomega.Equal("Failure"))
}

// TestUpdateStatusWithClientError tests status updates when the Kubernetes client returns an error.
// It verifies that client errors are properly handled and returned.
func TestUpdateStatusWithClientError(t *testing.T) {
	g := gomega.NewWithT(t)

	// Create a fake client that always returns an error
	errClient := &fakeErrorClient{err: errors.New("test error")}
	updater := controller.NewStatusUpdater(errClient)

	heartbeat := &monitoringv1alpha1.Heartbeat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-heartbeat",
			Namespace: "default",
		},
	}

	err := updater.UpdateStatus(context.Background(), heartbeat, 200, true, "test message")

	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(err.Error()).To(gomega.ContainSubstring("test error"))
}

// fakeErrorClient is a fake client that always returns an error for status updates.
// It's used to test error handling in status update operations.
type fakeErrorClient struct {
	client.Client
	err error
}

// Status returns a fake status writer that always returns an error.
func (c *fakeErrorClient) Status() client.StatusWriter {
	return &fakeErrorStatusWriter{err: c.err}
}

// fakeErrorStatusWriter is a fake status writer that always returns an error.
type fakeErrorStatusWriter struct {
	client.StatusWriter
	err error
}

// Update always returns the configured error.
func (w *fakeErrorStatusWriter) Update(_ context.Context, _ client.Object, _ ...client.SubResourceUpdateOption) error {
	return w.err
}
