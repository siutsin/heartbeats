package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1alpha1 "github.com/siutsin/heartbeats/api/v1alpha1"
	"github.com/siutsin/heartbeats/internal/logger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StatusUpdater handles updating the status of Heartbeat resources.
// It provides methods for updating various aspects of the Heartbeat status
// including health status, error conditions, and metadata.
type StatusUpdater struct {
	Client client.Client // Kubernetes client for updating resources
}

// NewStatusUpdater creates a new StatusUpdater instance with the provided Kubernetes client.
//
// Parameters:
//   - client: The Kubernetes client used for updating Heartbeat resources
//
// Returns:
//   - *StatusUpdater: A new StatusUpdater instance
func NewStatusUpdater(client client.Client) *StatusUpdater {
	return &StatusUpdater{
		Client: client,
	}
}

// UpdateStatus updates the status of a Heartbeat resource with the provided values.
// This is the core method that all other status update methods use internally.
//
// Parameters:
//   - ctx: Context for the operation
//   - heartbeat: The Heartbeat resource to update
//   - statusCode: HTTP status code from the health check (0 if not applicable)
//   - healthy: Whether the endpoint is considered healthy
//   - message: Status message describing the current state
//
// Returns:
//   - error: Non-nil if the status update failed, nil otherwise
func (u *StatusUpdater) UpdateStatus(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	statusCode int,
	healthy bool,
	message string,
) error {
	log := log.FromContext(ctx)

	heartbeat.Status.LastStatus = statusCode
	now := metav1.Now()
	heartbeat.Status.LastChecked = &now
	heartbeat.Status.Healthy = healthy
	heartbeat.Status.Message = message

	if err := u.Client.Status().Update(ctx, heartbeat); err != nil {
		logger.Error(log, ErrFailedToUpdateStatus, err, nil)
		return err
	}

	return nil
}

// UpdateSecretErrorStatus updates the status when there's an error retrieving the secret.
// This method logs the error and sets the status to indicate a secret-related failure.
//
// Parameters:
//   - ctx: Context for the operation
//   - heartbeat: The Heartbeat resource to update
//   - err: The error that occurred while retrieving the secret
//
// Returns:
//   - error: Non-nil if the status update failed, nil otherwise
func (u *StatusUpdater) UpdateSecretErrorStatus(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	err error,
) error {
	log := log.FromContext(ctx)
	logger.Error(log, ErrFailedToGetSecret, err, nil)
	return u.UpdateStatus(
		ctx,
		heartbeat,
		0,
		false,
		ErrFailedToGetSecret,
	)
}

// UpdateMissingKeyStatus updates the status when a required key is missing from the secret.
// This method logs the missing key and sets the status to indicate the configuration issue.
//
// Parameters:
//   - ctx: Context for the operation
//   - heartbeat: The Heartbeat resource to update
//   - key: The name of the missing key in the secret
//
// Returns:
//   - error: Non-nil if the status update failed, nil otherwise
func (u *StatusUpdater) UpdateMissingKeyStatus(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	key string,
) error {
	log := log.FromContext(ctx)
	logger.Error(log, ErrMissingRequiredKey, nil, map[string]interface{}{
		"key": key,
	})
	return u.UpdateStatus(
		ctx,
		heartbeat,
		0,
		false,
		ErrMissingRequiredKey,
	)
}

// UpdateEmptyEndpointStatus updates the status when the endpoint URL is empty.
// This method logs the issue and sets the status to indicate an empty endpoint configuration.
//
// Parameters:
//   - ctx: Context for the operation
//   - heartbeat: The Heartbeat resource to update
//
// Returns:
//   - error: Non-nil if the status update failed, nil otherwise
func (u *StatusUpdater) UpdateEmptyEndpointStatus(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
) error {
	log := log.FromContext(ctx)
	logger.Error(log, ErrEndpointNotSpecified, nil, nil)
	return u.UpdateStatus(
		ctx,
		heartbeat,
		0,
		false,
		ErrEndpointNotSpecified,
	)
}

// UpdateHealthCheckErrorStatus updates the status when there's an error during health checking.
// This method logs the error and sets the status to indicate a health check failure.
//
// Parameters:
//   - ctx: Context for the operation
//   - heartbeat: The Heartbeat resource to update
//   - statusCode: HTTP status code if available (0 if not applicable)
//   - err: The error that occurred during health checking
//
// Returns:
//   - error: Non-nil if the status update failed, nil otherwise
func (u *StatusUpdater) UpdateHealthCheckErrorStatus(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	statusCode int,
	err error,
) error {
	log := log.FromContext(ctx)
	logger.Error(log, ErrFailedToCheckEndpoint, err, nil)
	return u.UpdateStatus(
		ctx,
		heartbeat,
		statusCode,
		false,
		ErrFailedToCheckEndpoint,
	)
}

// UpdateInvalidRangeStatus updates the status when the status code range is invalid.
// This method logs the issue and sets the status to indicate an invalid configuration.
//
// Parameters:
//   - ctx: Context for the operation
//   - heartbeat: The Heartbeat resource to update
//   - statusCode: HTTP status code if available (0 if not applicable)
//
// Returns:
//   - error: Non-nil if the status update failed, nil otherwise
func (u *StatusUpdater) UpdateInvalidRangeStatus(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	statusCode int,
) error {
	log := log.FromContext(ctx)
	logger.Error(log, ErrInvalidStatusCodeRange, nil, nil)
	return u.UpdateStatus(
		ctx,
		heartbeat,
		statusCode,
		false,
		ErrInvalidStatusCodeRange,
	)
}

// UpdateHealthStatus updates the status based on the health check result.
// This method sets the status to reflect whether the endpoint is healthy or unhealthy,
// including the report status for monitoring systems.
//
// Parameters:
//   - ctx: Context for the operation
//   - heartbeat: The Heartbeat resource to update
//   - healthy: Whether the endpoint is considered healthy
//   - statusCode: HTTP status code returned by the endpoint
//   - err: Error from health checking (nil if successful)
//   - reportSuccess: Whether the report to monitoring systems was successful
//
// Returns:
//   - error: Non-nil if the status update failed, nil otherwise
func (u *StatusUpdater) UpdateHealthStatus(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	healthy bool,
	statusCode int,
	err error,
	reportSuccess bool,
) error {
	log := log.FromContext(ctx)

	now := metav1.Now()
	heartbeat.Status.LastChecked = &now
	heartbeat.Status.LastStatus = statusCode
	heartbeat.Status.Healthy = healthy

	if err != nil {
		heartbeat.Status.Message = err.Error()
	} else if healthy {
		heartbeat.Status.Message = ErrEndpointHealthy
	} else {
		heartbeat.Status.Message = ErrStatusCodeNotInRange
	}

	if reportSuccess {
		heartbeat.Status.ReportStatus = "Success"
	} else {
		heartbeat.Status.ReportStatus = "Failure"
	}

	if err := u.Client.Status().Update(ctx, heartbeat); err != nil {
		logger.Error(log, ErrFailedToUpdateStatus, err, nil)
		return err
	}

	return nil
}
