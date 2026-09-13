package heartbeats_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	monitoringv1alpha1 "github.com/siutsin/heartbeats/api/v1alpha1"
	heartbeats "github.com/siutsin/heartbeats/internal"
)

// setupScheme returns a scheme with the monitoring API types registered.
func setupScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := monitoringv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add monitoring API to scheme: %v", err)
	}
	return scheme
}

// createTestHeartbeat returns a test heartbeat with default metadata.
func createTestHeartbeat() *monitoringv1alpha1.Heartbeat {
	return &monitoringv1alpha1.Heartbeat{
		Name:      "test-heartbeat",
		Namespace: "default",
	}
}

// createTestClient returns a fake client holding heartbeat.
func createTestClient(t *testing.T, heartbeat *monitoringv1alpha1.Heartbeat) client.Client {
	t.Helper()

	return fake.NewClientBuilder().
		WithScheme(setupScheme(t)).
		WithObjects(heartbeat).
		WithStatusSubresource(heartbeat).
		Build()
}

// TestNewStatusUpdater verifies the factory returns a usable updater.
func TestNewStatusUpdater(t *testing.T) {

	client := fake.NewClientBuilder().WithScheme(setupScheme(t)).Build()
	updater := heartbeats.NewStatusUpdater(client)

	require.NotNil(t, updater)
	require.Equal(t, client, updater.Client)
}

// TestUpdateStatus_Basic verifies UpdateStatus sets every status field.
func TestUpdateStatus_Basic(t *testing.T) {

	heartbeat := &monitoringv1alpha1.Heartbeat{
		Name:      "test-heartbeat",
		Namespace: "default",
	}

	client := fake.NewClientBuilder().
		WithScheme(setupScheme(t)).
		WithObjects(heartbeat).
		WithStatusSubresource(heartbeat).
		Build()

	updater := heartbeats.NewStatusUpdater(client)

	err := updater.UpdateStatus(context.Background(), heartbeat, 200, true, "test message")

	require.NoError(t, err)
	require.Equal(t, 200, heartbeat.Status.LastStatus)
	require.True(t, heartbeat.Status.Healthy)
	require.Equal(t, "test message", heartbeat.Status.Message)
	require.NotNil(t, heartbeat.Status.LastChecked)
}

// errorConditionTestCase is one error-status row: the update call plus expected status.
type errorConditionTestCase struct {
	name            string
	updateFunc      func(*heartbeats.StatusUpdater, context.Context, *monitoringv1alpha1.Heartbeat) error
	expectedStatus  int
	expectedHealthy bool
	expectedMsg     string
}

// errorConditionTestCases defines the test cases for error condition status updates.
var errorConditionTestCases = []errorConditionTestCase{
	{
		name: "update secret error status",
		updateFunc: func(u *heartbeats.StatusUpdater, ctx context.Context, h *monitoringv1alpha1.Heartbeat) error {
			return u.UpdateSecretErrorStatus(ctx, h, errors.New("test error"))
		},
		expectedStatus:  0,
		expectedHealthy: false,
		expectedMsg:     heartbeats.ErrFailedToGetSecret,
	},
	{
		name: "update missing key status",
		updateFunc: func(u *heartbeats.StatusUpdater, ctx context.Context, h *monitoringv1alpha1.Heartbeat) error {
			return u.UpdateMissingKeyStatus(ctx, h, "test-key")
		},
		expectedStatus:  0,
		expectedHealthy: false,
		expectedMsg:     heartbeats.ErrMissingRequiredKey,
	},
	{
		name: "update empty endpoint status",
		updateFunc: func(u *heartbeats.StatusUpdater, ctx context.Context, h *monitoringv1alpha1.Heartbeat) error {
			return u.UpdateEmptyEndpointStatus(ctx, h)
		},
		expectedStatus:  0,
		expectedHealthy: false,
		expectedMsg:     heartbeats.ErrEndpointNotSpecified,
	},
	{
		name: "update health check error status",
		updateFunc: func(u *heartbeats.StatusUpdater, ctx context.Context, h *monitoringv1alpha1.Heartbeat) error {
			return u.UpdateHealthCheckErrorStatus(ctx, h, 500, errors.New("test error"))
		},
		expectedStatus:  500,
		expectedHealthy: false,
		expectedMsg:     heartbeats.ErrFailedToCheckEndpoint,
	},
	{
		name: "update invalid range status",
		updateFunc: func(u *heartbeats.StatusUpdater, ctx context.Context, h *monitoringv1alpha1.Heartbeat) error {
			return u.UpdateInvalidRangeStatus(ctx, h, 500)
		},
		expectedStatus:  500,
		expectedHealthy: false,
		expectedMsg:     heartbeats.ErrInvalidStatusCodeRange,
	},
}

// TestUpdateStatus_ErrorConditions verifies error paths set the expected unhealthy status.
func TestUpdateStatus_ErrorConditions(t *testing.T) {
	for _, tt := range errorConditionTestCases {
		t.Run(tt.name, func(t *testing.T) {
			runErrorConditionTest(t, tt)
		})
	}
}

// runErrorConditionTest executes one errorConditionTestCase.
func runErrorConditionTest(t *testing.T, tt errorConditionTestCase) {

	heartbeat := createTestHeartbeat()
	client := createTestClient(t, heartbeat)
	updater := heartbeats.NewStatusUpdater(client)

	err := tt.updateFunc(updater, context.Background(), heartbeat)

	require.NoError(t, err)
	require.Equal(t, tt.expectedStatus, heartbeat.Status.LastStatus)
	require.Equal(t, tt.expectedHealthy, heartbeat.Status.Healthy)
	require.Equal(t, tt.expectedMsg, heartbeat.Status.Message)
	require.NotNil(t, heartbeat.Status.LastChecked)
}

// TestUpdateHealthStatus_HealthyEndpoint verifies healthy results set success status.
func TestUpdateHealthStatus_HealthyEndpoint(t *testing.T) {

	heartbeat := createTestHeartbeat()
	client := createTestClient(t, heartbeat)
	updater := heartbeats.NewStatusUpdater(client)

	err := updater.UpdateHealthStatus(
		context.Background(),
		heartbeat,
		true, // healthy
		200,  // status code
		nil,  // no error
		true, // report success
	)

	require.NoError(t, err)
	require.Equal(t, 200, heartbeat.Status.LastStatus)
	require.True(t, heartbeat.Status.Healthy)
	require.Equal(t, heartbeats.ErrEndpointHealthy, heartbeat.Status.Message)
	require.Equal(t, "Success", heartbeat.Status.ReportStatus)
}

// TestUpdateHealthStatus_UnhealthyEndpoint verifies unhealthy results set failure status.
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
			expectedMsg: heartbeats.ErrStatusCodeNotInRange,
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
			heartbeat := createTestHeartbeat()
			client := createTestClient(t, heartbeat)
			updater := heartbeats.NewStatusUpdater(client)

			err := updater.UpdateHealthStatus(
				context.Background(),
				heartbeat,
				false, // unhealthy
				tt.statusCode,
				tt.err,
				true, // report success
			)

			require.NoError(t, err)
			require.Equal(t, tt.statusCode, heartbeat.Status.LastStatus)
			require.False(t, heartbeat.Status.Healthy)
			require.Equal(t, tt.expectedMsg, heartbeat.Status.Message)
			require.Equal(t, "Success", heartbeat.Status.ReportStatus)
		})
	}
}

// TestUpdateHealthStatus_ReportFailure verifies a failed report sets ReportStatus Failure.
func TestUpdateHealthStatus_ReportFailure(t *testing.T) {

	heartbeat := createTestHeartbeat()
	client := createTestClient(t, heartbeat)
	updater := heartbeats.NewStatusUpdater(client)

	err := updater.UpdateHealthStatus(
		context.Background(),
		heartbeat,
		true,  // healthy
		200,   // status code
		nil,   // no error
		false, // report failure
	)

	require.NoError(t, err)
	require.Equal(t, 200, heartbeat.Status.LastStatus)
	require.True(t, heartbeat.Status.Healthy)
	require.Equal(t, heartbeats.ErrEndpointHealthy, heartbeat.Status.Message)
	require.Equal(t, "Failure", heartbeat.Status.ReportStatus)
}

// TestUpdateStatusWithClientError verifies client failures surface to the caller.
func TestUpdateStatusWithClientError(t *testing.T) {

	errClient := &fakeErrorClient{err: errors.New("test error")}
	updater := heartbeats.NewStatusUpdater(errClient)

	heartbeat := &monitoringv1alpha1.Heartbeat{
		Name:      "test-heartbeat",
		Namespace: "default",
	}

	err := updater.UpdateStatus(context.Background(), heartbeat, 200, true, "test message")

	require.Error(t, err)
	require.Contains(t, err.Error(), "test error")
}

// fakeErrorClient is a client whose status writes always fail.
type fakeErrorClient struct {
	client.Client
	err error
}

// Status returns the failing writer.
func (c *fakeErrorClient) Status() client.StatusWriter {
	return &fakeErrorStatusWriter{err: c.err}
}

// fakeErrorStatusWriter is a status writer that always fails.
type fakeErrorStatusWriter struct {
	client.StatusWriter
	err error
}

// Update always returns the configured error.
func (w *fakeErrorStatusWriter) Update(_ context.Context, _ client.Object, _ ...client.SubResourceUpdateOption) error {
	return w.err
}
