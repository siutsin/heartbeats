package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1alpha1 "github.com/siutsin/heartbeats/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StatusUpdater handles updating the status of Heartbeat resources
type StatusUpdater struct {
	Client client.Client
}

// NewStatusUpdater creates a new StatusUpdater
func NewStatusUpdater(client client.Client) *StatusUpdater {
	return &StatusUpdater{
		Client: client,
	}
}

// UpdateStatus updates the status of a Heartbeat resource
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
		log.Error(err, ErrFailedToUpdateStatus)
		return err
	}

	return nil
}

// UpdateSecretErrorStatus updates the status when there's an error with the secret
func (u *StatusUpdater) UpdateSecretErrorStatus(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	err error,
) error {
	log := log.FromContext(ctx)
	log.Error(err, ErrFailedToGetSecret)
	return u.UpdateStatus(
		ctx,
		heartbeat,
		0,
		false,
		ErrFailedToGetSecret,
	)
}

// UpdateMissingKeyStatus updates the status when a required key is missing
func (u *StatusUpdater) UpdateMissingKeyStatus(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	key string,
) error {
	log := log.FromContext(ctx)
	log.Error(nil, ErrMissingRequiredKey, "key", key)
	return u.UpdateStatus(
		ctx,
		heartbeat,
		0,
		false,
		ErrMissingRequiredKey,
	)
}

// UpdateEmptyEndpointStatus updates the status when the endpoint is empty
func (u *StatusUpdater) UpdateEmptyEndpointStatus(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
) error {
	log := log.FromContext(ctx)
	log.Error(nil, ErrEndpointNotSpecified)
	return u.UpdateStatus(
		ctx,
		heartbeat,
		0,
		false,
		ErrEndpointNotSpecified,
	)
}

// UpdateHealthCheckErrorStatus updates the status when there's an error checking health
func (u *StatusUpdater) UpdateHealthCheckErrorStatus(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	statusCode int,
	err error,
) error {
	log := log.FromContext(ctx)
	log.Error(err, ErrFailedToCheckEndpoint)
	return u.UpdateStatus(
		ctx,
		heartbeat,
		statusCode,
		false,
		ErrFailedToCheckEndpoint,
	)
}

// UpdateInvalidRangeStatus updates the status when the status code range is invalid
func (u *StatusUpdater) UpdateInvalidRangeStatus(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	statusCode int,
) error {
	log := log.FromContext(ctx)
	log.Error(nil, ErrInvalidStatusCodeRange)
	return u.UpdateStatus(
		ctx,
		heartbeat,
		statusCode,
		false,
		ErrInvalidStatusCodeRange,
	)
}

// UpdateHealthStatus updates the status based on the health check result
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
		log.Error(err, ErrFailedToUpdateStatus)
		return err
	}

	return nil
}
