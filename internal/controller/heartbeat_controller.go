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

package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1alpha1 "github.com/siutsin/heartbeats/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// K8sClient is the Kubernetes client used by the controller
var K8sClient client.Client

// HeartbeatReconciler reconciles a Heartbeat object
type HeartbeatReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Config        Config
	HealthChecker HealthChecker
	StatusUpdater *StatusUpdater
}

// +kubebuilder:rbac:groups=monitoring.siutsin.com,resources=heartbeats,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.siutsin.com,resources=heartbeats/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=monitoring.siutsin.com,resources=heartbeats/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop
func (r *HeartbeatReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Step 1: Fetch the Heartbeat resource
	heartbeat, err := r.fetchHeartbeat(ctx, req)
	if err != nil {
		return ctrl.Result{}, err
	}
	if heartbeat == nil {
		// Heartbeat was deleted, nothing to reconcile
		return ctrl.Result{}, nil
	}

	// Step 2: Fetch and validate the secret containing endpoints
	secret, err := r.fetchAndValidateSecret(ctx, req, heartbeat)
	if err != nil {
		return r.handleReconcileError(ctx, heartbeat, err)
	}

	// Step 3: Extract and validate the target endpoint
	targetEndpoint, err := r.extractTargetEndpoint(ctx, heartbeat, secret)
	if err != nil {
		return r.handleReconcileError(ctx, heartbeat, err)
	}

	// Step 4: Extract report endpoints for health status reporting
	reportEndpointsSecret, err := r.extractReportEndpoints(ctx, heartbeat, secret)
	if err != nil {
		return r.handleReconcileError(ctx, heartbeat, err)
	}

	// Step 5: Perform health check and report status
	if err := r.performHealthCheckAndReport(ctx, heartbeat, targetEndpoint, reportEndpointsSecret); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.Config.RequeueAfter}, nil
}

// handleReconcileError handles errors during reconciliation by updating status and returning requeue result.
// This centralises the error handling pattern used across multiple reconciliation steps.
func (r *HeartbeatReconciler) handleReconcileError(
	_ context.Context,
	_ *monitoringv1alpha1.Heartbeat,
	_ error,
) (ctrl.Result, error) {
	// Status is already updated by the calling method, just return requeue
	return ctrl.Result{RequeueAfter: r.Config.RequeueAfter}, nil
}

// fetchHeartbeat retrieves the Heartbeat resource from the cluster.
// Returns nil if the resource was not found (deleted), or an error if the fetch failed.
func (r *HeartbeatReconciler) fetchHeartbeat(
	ctx context.Context,
	req ctrl.Request,
) (*monitoringv1alpha1.Heartbeat, error) {
	var heartbeat monitoringv1alpha1.Heartbeat
	if err := r.Get(ctx, req.NamespacedName, &heartbeat); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil // Resource was deleted
		}
		return nil, client.IgnoreNotFound(err)
	}
	return &heartbeat, nil
}

// fetchAndValidateSecret retrieves the secret containing endpoint configuration.
// Returns an error if the secret cannot be fetched, which will trigger status update and requeue.
func (r *HeartbeatReconciler) fetchAndValidateSecret(
	ctx context.Context,
	req ctrl.Request,
	heartbeat *monitoringv1alpha1.Heartbeat,
) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	secretNamespace := heartbeat.Spec.EndpointsSecret.Namespace
	if secretNamespace == "" {
		secretNamespace = req.Namespace
	}

	secretKey := client.ObjectKey{
		Namespace: secretNamespace,
		Name:      heartbeat.Spec.EndpointsSecret.Name,
	}

	if err := r.Get(ctx, secretKey, secret); err != nil {
		if err := r.StatusUpdater.UpdateSecretErrorStatus(ctx, heartbeat, err); err != nil {
			return nil, err
		}
		return nil, err
	}

	return secret, nil
}

// extractTargetEndpoint extracts and validates the target endpoint URL from the secret.
// Returns an error if the target endpoint key is missing or empty, which will trigger status update and requeue.
func (r *HeartbeatReconciler) extractTargetEndpoint(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	secret *corev1.Secret,
) (string, error) {
	endpoint, err := r.extractEndpointFromSecret(ctx, heartbeat, secret, heartbeat.Spec.EndpointsSecret.TargetEndpointKey)
	if err != nil {
		return "", err
	}

	if endpoint == "" {
		if err := r.StatusUpdater.UpdateEmptyEndpointStatus(ctx, heartbeat); err != nil {
			return "", err
		}
		return "", fmt.Errorf("target endpoint is empty")
	}

	return endpoint, nil
}

// extractReportEndpoints extracts the healthy and unhealthy endpoint URLs from the secret for reporting.
// Returns an error if either report endpoint key is missing, which will trigger status update and requeue.
func (r *HeartbeatReconciler) extractReportEndpoints(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	secret *corev1.Secret,
) (monitoringv1alpha1.EndpointsSecret, error) {
	// Extract healthy endpoint
	healthyEndpoint, err := r.extractEndpointFromSecret(
		ctx, heartbeat, secret, heartbeat.Spec.EndpointsSecret.HealthyEndpointKey,
	)
	if err != nil {
		return monitoringv1alpha1.EndpointsSecret{}, err
	}

	// Extract unhealthy endpoint
	unhealthyEndpoint, err := r.extractEndpointFromSecret(
		ctx, heartbeat, secret, heartbeat.Spec.EndpointsSecret.UnhealthyEndpointKey,
	)
	if err != nil {
		return monitoringv1alpha1.EndpointsSecret{}, err
	}

	// Create EndpointsSecret with actual URLs for reporting
	reportEndpointsSecret := monitoringv1alpha1.EndpointsSecret{
		HealthyEndpointKey:   healthyEndpoint,
		UnhealthyEndpointKey: unhealthyEndpoint,
	}

	return reportEndpointsSecret, nil
}

// extractEndpointFromSecret extracts an endpoint URL from the secret by key.
// Returns an error if the key is missing, which will trigger status update.
func (r *HeartbeatReconciler) extractEndpointFromSecret(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	secret *corev1.Secret,
	key string,
) (string, error) {
	endpointBytes, ok := secret.Data[key]
	if !ok {
		if err := r.StatusUpdater.UpdateMissingKeyStatus(ctx, heartbeat, key); err != nil {
			return "", err
		}
		return "", fmt.Errorf("missing endpoint key: %s", key)
	}

	return string(endpointBytes), nil
}

// performHealthCheckAndReport performs the actual health check and reports the status.
// This is the core business logic that determines if the endpoint is healthy and reports accordingly.
func (r *HeartbeatReconciler) performHealthCheckAndReport(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	targetEndpoint string,
	reportEndpointsSecret monitoringv1alpha1.EndpointsSecret,
) error {
	// Validate status code ranges before health check
	if r.validateStatusCodeRanges(ctx, heartbeat) {
		return nil // Early return, status already updated
	}

	// Perform the health check
	healthy, statusCode, reportSuccess, err := r.HealthChecker.CheckEndpointHealth(
		ctx,
		targetEndpoint,
		heartbeat.Spec.ExpectedStatusCodeRanges,
		reportEndpointsSecret,
	)

	if err != nil {
		return r.handleHealthCheckError(ctx, heartbeat, statusCode, err)
	}

	// Update status with successful health check results
	return r.updateHealthStatus(ctx, heartbeat, healthy, statusCode, reportSuccess)
}

// handleHealthCheckError handles errors during health checks by updating status and returning the error.
func (r *HeartbeatReconciler) handleHealthCheckError(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	statusCode int,
	err error,
) error {
	if updateErr := r.StatusUpdater.UpdateHealthCheckErrorStatus(ctx, heartbeat, statusCode, err); updateErr != nil {
		return fmt.Errorf("failed to update status after health check error: %w", updateErr)
	}
	return fmt.Errorf("health check failed: %w", err)
}

// updateHealthStatus updates the heartbeat status with health check results.
func (r *HeartbeatReconciler) updateHealthStatus(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	healthy bool,
	statusCode int,
	reportSuccess bool,
) error {
	if err := r.StatusUpdater.UpdateHealthStatus(ctx, heartbeat, healthy, statusCode, nil, reportSuccess); err != nil {
		return fmt.Errorf("failed to update health status: %w", err)
	}
	return nil
}

// validateStatusCodeRanges validates that all status code ranges have valid min/max values.
// Returns true if an invalid range was found and status was updated.
func (r *HeartbeatReconciler) validateStatusCodeRanges(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
) bool {
	for _, statusRange := range heartbeat.Spec.ExpectedStatusCodeRanges {
		if statusRange.Min > statusRange.Max {
			if err := r.StatusUpdater.UpdateInvalidRangeStatus(
				ctx,
				heartbeat,
				0, // No status code available before health check
			); err != nil {
				// Log error but don't fail the reconciliation
				log.FromContext(ctx).Error(err, "Failed to update invalid range status")
			}
			return true // Invalid range found
		}
	}
	return false // No invalid ranges found
}

// SetupWithManager sets up the controller with the Manager.
func (r *HeartbeatReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Config.DefaultTimeout == 0 {
		r.Config = DefaultConfig()
	}
	r.HealthChecker = NewHealthChecker(r.Config)
	r.StatusUpdater = NewStatusUpdater(r.Client)
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.Heartbeat{}).
		Complete(r)
}
