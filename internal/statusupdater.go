package heartbeats

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1alpha1 "github.com/siutsin/heartbeats/api/v1alpha1"
)

// StatusUpdater writes Heartbeat status updates through a Kubernetes client.
type StatusUpdater struct {
	Client client.Client
}

// NewStatusUpdater returns a StatusUpdater that writes through client.
func NewStatusUpdater(client client.Client) *StatusUpdater {
	return &StatusUpdater{
		Client: client,
	}
}

// UpdateStatus writes statusCode, healthy, and message to the Heartbeat status.
// It is the core method behind the other helpers; statusCode is 0 when not applicable.
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

// UpdateSecretErrorStatus marks the Heartbeat unhealthy after a secret fetch failure.
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

// UpdateMissingKeyStatus marks the Heartbeat unhealthy when a secret key is missing.
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

// UpdateEmptyEndpointStatus marks the Heartbeat unhealthy when the endpoint is empty.
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

// UpdateHealthCheckErrorStatus marks the Heartbeat unhealthy after a health check failure.
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

// UpdateInvalidRangeStatus marks the Heartbeat unhealthy for an invalid status code range.
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

// UpdateHealthStatus records a health check result, including the report outcome.
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
