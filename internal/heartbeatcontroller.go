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

package heartbeats

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1alpha1 "github.com/siutsin/heartbeats/api/v1alpha1"
)

const (
	logNameHeartbeatReconciler  = "heartbeat-reconciler"
	errMsgFailedToFetchResource = "Failed to fetch resource"
)

// Reconciler reconciles a Heartbeat object
type Reconciler struct {
	client.Client
	Config        Config
	HealthChecker HealthChecker
	StatusUpdater *StatusUpdater
}

// ParseInterval parses the interval string from the heartbeat spec into a time.Duration.
func ParseInterval(interval string) (time.Duration, error) {
	duration, err := time.ParseDuration(interval)
	if err != nil {
		return 0, fmt.Errorf("failed to parse interval '%s': %w", interval, err)
	}
	return duration, nil
}

// +kubebuilder:rbac:groups=monitoring.siutsin.com,resources=heartbeats,verbs=get;list;watch
// +kubebuilder:rbac:groups=monitoring.siutsin.com,resources=heartbeats/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx).WithName(logNameHeartbeatReconciler).WithValues(
		"namespace", req.Namespace, "name", req.Name,
	)

	heartbeat, err := r.fetchHeartbeat(ctx, req)
	if err != nil {
		l.Error(err, errMsgFailedToFetchResource)
		return ctrl.Result{}, err
	}
	if heartbeat == nil {
		l.Info("Resource not found, likely deleted")
		return ctrl.Result{}, nil
	}

	interval, err := ParseInterval(heartbeat.Spec.Interval)
	if err != nil {
		l.Error(err, "Failed to parse interval", "interval", heartbeat.Spec.Interval)
		interval = r.Config.RequeueAfter
	}

	if processErr := r.processHeartbeat(ctx, heartbeat, req, l); processErr != nil {
		l.Error(processErr, "Reconciliation error occurred", "requeue_after", interval)
		return ctrl.Result{RequeueAfter: interval}, nil
	}
	return ctrl.Result{RequeueAfter: interval}, nil
}

func (r *Reconciler) processHeartbeat(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	req ctrl.Request,
	l logr.Logger,
) error {
	secret, err := r.fetchAndValidateSecret(ctx, req, heartbeat, l)
	if err != nil {
		return err
	}

	targetEndpoint, err := r.extractTargetEndpoint(ctx, heartbeat, secret, l)
	if err != nil {
		return err
	}

	reportEndpoints, err := r.extractReportEndpoints(ctx, heartbeat, secret, l)
	if err != nil {
		return err
	}

	return r.performHealthCheckAndReport(ctx, heartbeat, targetEndpoint, reportEndpoints, l)
}

func (r *Reconciler) fetchHeartbeat(
	ctx context.Context,
	req ctrl.Request,
) (*monitoringv1alpha1.Heartbeat, error) {
	var heartbeat monitoringv1alpha1.Heartbeat
	if err := r.Get(ctx, req.NamespacedName, &heartbeat); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, client.IgnoreNotFound(err)
	}
	return &heartbeat, nil
}

func (r *Reconciler) fetchAndValidateSecret(
	ctx context.Context,
	req ctrl.Request,
	heartbeat *monitoringv1alpha1.Heartbeat,
	l logr.Logger,
) (*corev1.Secret, error) {
	secretNamespace := heartbeat.Spec.EndpointsSecret.Namespace
	if secretNamespace == "" {
		secretNamespace = req.Namespace
	}

	secret := &corev1.Secret{}
	if getErr := r.Get(ctx, client.ObjectKey{
		Namespace: secretNamespace,
		Name:      heartbeat.Spec.EndpointsSecret.Name,
	}, secret); getErr != nil {
		if updateErr := r.StatusUpdater.UpdateSecretErrorStatus(ctx, heartbeat, getErr); updateErr != nil {
			l.Error(updateErr, "Failed to update secret error status")
			return nil, updateErr
		}
		return nil, getErr
	}
	return secret, nil
}

func (r *Reconciler) extractTargetEndpoint(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	secret *corev1.Secret,
	l logr.Logger,
) (string, error) {
	endpoint, err := r.extractEndpointFromSecret(ctx, heartbeat, secret, heartbeat.Spec.EndpointsSecret.TargetEndpointKey, l)
	if err != nil {
		return "", err
	}
	if endpoint == "" {
		if updateErr := r.StatusUpdater.UpdateEmptyEndpointStatus(ctx, heartbeat); updateErr != nil {
			l.Error(updateErr, "Failed to update empty endpoint status")
			return "", updateErr
		}
		return "", fmt.Errorf("target endpoint is empty")
	}
	return endpoint, nil
}

func (r *Reconciler) extractEndpointFromSecret(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	secret *corev1.Secret,
	key string,
	l logr.Logger,
) (string, error) {
	endpointBytes, ok := secret.Data[key]
	if !ok {
		if updateErr := r.StatusUpdater.UpdateMissingKeyStatus(ctx, heartbeat, key); updateErr != nil {
			l.Error(updateErr, "Failed to update missing key status", "key", key)
			return "", updateErr
		}
		return "", fmt.Errorf("missing endpoint key: %s", key)
	}
	return string(endpointBytes), nil
}

func (r *Reconciler) extractReportEndpoints(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	secret *corev1.Secret,
	l logr.Logger,
) (monitoringv1alpha1.EndpointsSecret, error) {
	healthyEndpoint, err := r.extractEndpointFromSecret(
		ctx, heartbeat, secret, heartbeat.Spec.EndpointsSecret.HealthyEndpointKey, l,
	)
	if err != nil {
		return monitoringv1alpha1.EndpointsSecret{}, err
	}
	unhealthyEndpoint, err := r.extractEndpointFromSecret(
		ctx, heartbeat, secret, heartbeat.Spec.EndpointsSecret.UnhealthyEndpointKey, l,
	)
	if err != nil {
		return monitoringv1alpha1.EndpointsSecret{}, err
	}
	return monitoringv1alpha1.EndpointsSecret{
		HealthyEndpointKey:   healthyEndpoint,
		UnhealthyEndpointKey: unhealthyEndpoint,
	}, nil
}

func (r *Reconciler) performHealthCheckAndReport(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	targetEndpoint string,
	reportEndpoints monitoringv1alpha1.EndpointsSecret,
	l logr.Logger,
) error {
	if r.validateStatusCodeRanges(ctx, heartbeat, l) {
		return nil
	}

	healthy, statusCode, reportSuccess, err := r.HealthChecker.CheckEndpointHealth(
		ctx,
		targetEndpoint,
		heartbeat.Spec.ExpectedStatusCodeRanges,
		reportEndpoints,
	)
	if err != nil {
		if updateErr := r.StatusUpdater.UpdateHealthCheckErrorStatus(ctx, heartbeat, statusCode, err); updateErr != nil {
			l.Error(updateErr, "Failed to update status after health check error")
			return fmt.Errorf("failed to update status after health check error: %w", updateErr)
		}
		return fmt.Errorf("health check failed: %w", err)
	}

	if err := r.StatusUpdater.UpdateHealthStatus(ctx, heartbeat, healthy, statusCode, nil, reportSuccess); err != nil {
		return fmt.Errorf("failed to update health status: %w", err)
	}
	return nil
}

func (r *Reconciler) validateStatusCodeRanges(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	l logr.Logger,
) bool {
	for i, statusRange := range heartbeat.Spec.ExpectedStatusCodeRanges {
		if statusRange.Min > statusRange.Max {
			l.Error(fmt.Errorf("invalid status code range: min %d > max %d", statusRange.Min, statusRange.Max),
				"Invalid status code range", "range_index", i)
			if updateErr := r.StatusUpdater.UpdateInvalidRangeStatus(ctx, heartbeat, 0); updateErr != nil {
				l.Error(updateErr, "Failed to update invalid range status")
			}
			return true
		}
	}
	return false
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Config.DefaultTimeout == 0 {
		r.Config = DefaultConfig()
	}
	r.HealthChecker = NewHealthChecker(r.Config)
	r.StatusUpdater = NewStatusUpdater(r.Client)

	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.Heartbeat{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: MaxConcurrentReconciles,
		}).
		Complete(r)
}
