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

func setupScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = monitoringv1alpha1.AddToScheme(scheme)
	return scheme
}

func TestNewStatusUpdater(t *testing.T) {
	g := gomega.NewWithT(t)

	client := fake.NewClientBuilder().WithScheme(setupScheme()).Build()
	updater := controller.NewStatusUpdater(client)

	g.Expect(updater).NotTo(gomega.BeNil())
	g.Expect(updater.Client).To(gomega.Equal(client))
}

func TestStatusUpdates(t *testing.T) {
	tests := []struct {
		name            string
		updateFunc      func(*controller.StatusUpdater, context.Context, *monitoringv1alpha1.Heartbeat) error
		expectedStatus  int
		expectedHealthy bool
		expectedMsg     string
	}{
		{
			name: "update status",
			updateFunc: func(u *controller.StatusUpdater, ctx context.Context, h *monitoringv1alpha1.Heartbeat) error {
				return u.UpdateStatus(ctx, h, 200, true, "test message")
			},
			expectedStatus:  200,
			expectedHealthy: true,
			expectedMsg:     "test message",
		},
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

func TestUpdateHealthStatus(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		healthy     bool
		err         error
		expectedMsg string
	}{
		{
			name:        "healthy endpoint",
			statusCode:  200,
			healthy:     true,
			err:         nil,
			expectedMsg: controller.ErrEndpointHealthy,
		},
		{
			name:        "status code not in range",
			statusCode:  500,
			healthy:     false,
			err:         nil,
			expectedMsg: controller.ErrStatusCodeNotInRange,
		},
		{
			name:        "error checking health",
			statusCode:  0,
			healthy:     false,
			err:         errors.New("connection refused"),
			expectedMsg: "connection refused",
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

			err := updater.UpdateHealthStatus(
				context.Background(),
				heartbeat,
				tt.healthy,
				tt.statusCode,
				tt.err,
			)

			g.Expect(err).NotTo(gomega.HaveOccurred())
			g.Expect(heartbeat.Status.LastStatus).To(gomega.Equal(tt.statusCode))
			g.Expect(heartbeat.Status.Healthy).To(gomega.Equal(tt.healthy))
			g.Expect(heartbeat.Status.Message).To(gomega.Equal(tt.expectedMsg))
		})
	}
}

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

	err := updater.UpdateStatus(
		context.Background(),
		heartbeat,
		200,
		true,
		"test message",
	)

	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(err.Error()).To(gomega.ContainSubstring("test error"))
}

// fakeErrorClient is a fake client that always returns an error
type fakeErrorClient struct {
	client.Client
	err error
}

func (c *fakeErrorClient) Status() client.StatusWriter {
	return &fakeErrorStatusWriter{err: c.err}
}

type fakeErrorStatusWriter struct {
	client.StatusWriter
	err error
}

func (w *fakeErrorStatusWriter) Update(_ context.Context, _ client.Object, _ ...client.SubResourceUpdateOption) error {
	return w.err
}
