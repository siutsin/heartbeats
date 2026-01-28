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
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	monitoringv1alpha1 "github.com/siutsin/heartbeats/api/v1alpha1"
	"github.com/siutsin/heartbeats/internal/logger"
	corev1 "k8s.io/api/core/v1"
)

const (
	logNameHeartbeatReconciler  = "heartbeat-reconciler"
	errMsgFailedToFetchResource = "Failed to fetch resource"
)

// HeartbeatReconciler reconciles a Heartbeat object
type HeartbeatReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Config        Config
	HealthChecker HealthChecker
	StatusUpdater *StatusUpdater
}

// ParseInterval parses the interval string from the heartbeat spec into a time.Duration.
// The interval string should match the pattern ^([0-9]+(s|m|h))$ (e.g., "30s", "5m", "1h").
func ParseInterval(interval string) (time.Duration, error) {
	duration, err := time.ParseDuration(interval)
	if err != nil {
		return 0, fmt.Errorf("failed to parse interval '%s': %w", interval, err)
	}
	return duration, nil
}

// +kubebuilder:rbac:groups=monitoring.siutsin.com,resources=heartbeats,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.siutsin.com,resources=heartbeats/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=monitoring.siutsin.com,resources=heartbeats/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop
func (r *HeartbeatReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reconcileLog := logger.WithRequest(ctx, logNameHeartbeatReconciler, req.Namespace, req.Name)
	logger.Info(reconcileLog, "Starting reconciliation", nil)

	// Step 1: Fetch the Heartbeat resource
	heartbeat, err := r.fetchHeartbeat(ctx, req)
	if err != nil {
		logger.Error(reconcileLog, errMsgFailedToFetchResource, err, nil)
		return ctrl.Result{}, err
	}
	if heartbeat == nil {
		logger.Info(reconcileLog, "Resource not found, likely deleted", nil)
		return ctrl.Result{}, nil
	}

	// Create heartbeat context for consistent logging
	heartbeatLog := logger.WithHeartbeat(ctx, logNameHeartbeatReconciler, heartbeat.Namespace, heartbeat.Name)

	// Parse the interval from the heartbeat spec
	interval, err := ParseInterval(heartbeat.Spec.Interval)
	if err != nil {
		logger.Error(heartbeatLog, "Failed to parse interval", err, map[string]interface{}{
			"interval": heartbeat.Spec.Interval,
		})
		// Fall back to config default on parsing error
		interval = r.Config.RequeueAfter
	}

	// Step 2: Process the heartbeat resource
	if processErr := r.processHeartbeat(ctx, heartbeat, req, heartbeatLog); processErr != nil {
		return r.handleReconcileError(ctx, heartbeat, processErr, interval)
	}

	completionFields := map[string]interface{}{
		"requeue_after": interval,
		"interval":      heartbeat.Spec.Interval,
	}
	logger.Info(heartbeatLog, "Reconciliation completed successfully", completionFields)
	return ctrl.Result{RequeueAfter: interval}, nil
}

// runStep is a generic helper for running a reconciliation step with logging.
func runStep[T any](
	log logr.Logger,
	name string,
	f func() (T, error),
) (T, error) {
	logger.Info(log, "Starting step", map[string]interface{}{"step": name})
	out, err := f()
	if err != nil {
		logger.Error(log, "Step failed", err, map[string]interface{}{"step": name})
	} else {
		logger.Info(log, "Step completed", map[string]interface{}{"step": name})
	}
	return out, err
}

// processHeartbeat handles the main reconciliation logic for a heartbeat resource.
func (r *HeartbeatReconciler) processHeartbeat(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	req ctrl.Request,
	heartbeatLog logr.Logger,
) error {
	logger.Info(heartbeatLog, "Resource fetched successfully", map[string]interface{}{
		"target_endpoint_key":    heartbeat.Spec.EndpointsSecret.TargetEndpointKey,
		"healthy_endpoint_key":   heartbeat.Spec.EndpointsSecret.HealthyEndpointKey,
		"unhealthy_endpoint_key": heartbeat.Spec.EndpointsSecret.UnhealthyEndpointKey,
	})

	// Step 2: Fetch and validate the secret containing endpoints
	secret, err := runStep(heartbeatLog, "fetchSecret", func() (*corev1.Secret, error) {
		l := log.FromContext(ctx).WithName(logNameHeartbeatReconciler)
		fetchedSecret, fetchErr := r.fetchAndValidateSecret(ctx, req, heartbeat, l)
		if fetchErr != nil {
			l.Error(fetchErr, errMsgFailedToFetchResource,
				"secret_name", heartbeat.Spec.EndpointsSecret.Name,
				"secret_namespace", heartbeat.Spec.EndpointsSecret.Namespace,
			)
			return nil, fetchErr
		}
		l.Info("Secret fetched successfully",
			"secret_namespace", heartbeat.Spec.EndpointsSecret.Namespace,
			"secret_name", heartbeat.Spec.EndpointsSecret.Name,
		)
		return fetchedSecret, nil
	})
	if err != nil {
		return err
	}

	// Step 3: Extract and validate the target endpoint
	targetEndpoint, err := runStep(heartbeatLog, "extractTargetEndpoint", func() (string, error) {
		l := log.FromContext(ctx).WithName(logNameHeartbeatReconciler)
		endpoint, extractErr := r.extractTargetEndpoint(ctx, heartbeat, secret, l)
		if extractErr != nil {
			l.Error(extractErr, "Failed to extract endpoint",
				"key", heartbeat.Spec.EndpointsSecret.TargetEndpointKey,
			)
			return "", extractErr
		}
		l.Info("Endpoint extracted successfully",
			"key", heartbeat.Spec.EndpointsSecret.TargetEndpointKey,
			"endpoint", endpoint,
		)
		return endpoint, nil
	})
	if err != nil {
		return err
	}

	// Step 4: Extract report endpoints for health status reporting
	reportEndpointsSecret, err := runStep(
		heartbeatLog,
		"extractReportEndpoints",
		func() (monitoringv1alpha1.EndpointsSecret, error) {
			endpoints, extractErr := r.extractReportEndpoints(ctx, heartbeat, secret)
			if extractErr != nil {
				logger.Error(heartbeatLog, errMsgFailedToFetchResource, extractErr, nil)
				return monitoringv1alpha1.EndpointsSecret{}, extractErr
			}
			reportFields := map[string]interface{}{
				"healthy_endpoint":   endpoints.HealthyEndpointKey,
				"unhealthy_endpoint": endpoints.UnhealthyEndpointKey,
			}
			logger.Info(heartbeatLog, "Report endpoints extracted successfully", reportFields)
			return endpoints, nil
		},
	)
	if err != nil {
		return err
	}

	// Step 5: Perform health check and report status
	_, err = runStep(heartbeatLog, "healthCheckAndReport", func() (struct{}, error) {
		if checkErr := r.performHealthCheckAndReport(ctx, heartbeat, targetEndpoint, reportEndpointsSecret); checkErr != nil {
			healthCheckFields := map[string]interface{}{
				"endpoint": targetEndpoint,
			}
			logger.Error(
				heartbeatLog,
				"Failed to perform health check and report",
				checkErr,
				healthCheckFields,
			)
			return struct{}{}, checkErr
		}
		return struct{}{}, nil
	})
	return err
}

// handleReconcileError handles errors during reconciliation by updating status and returning requeue result.
// This centralises the error handling pattern used across multiple reconciliation steps.
func (r *HeartbeatReconciler) handleReconcileError(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	err error,
	interval time.Duration,
) (ctrl.Result, error) {
	l := logger.WithHeartbeat(ctx, logNameHeartbeatReconciler, heartbeat.Namespace, heartbeat.Name)

	logger.Error(l, "Reconciliation error occurred", err, map[string]interface{}{
		"requeue_after": interval,
	})

	// Status is already updated by the calling method, just return requeue
	return ctrl.Result{RequeueAfter: interval}, nil
}

// fetchHeartbeat retrieves the Heartbeat resource from the cluster.
// Returns nil if the resource was not found (deleted), or an error if the fetch failed.
func (r *HeartbeatReconciler) fetchHeartbeat(
	ctx context.Context,
	req ctrl.Request,
) (*monitoringv1alpha1.Heartbeat, error) {
	l := log.FromContext(ctx).WithName(logNameHeartbeatReconciler)

	l.V(2).Info("Fetching heartbeat resource",
		"namespace", req.Namespace,
		"name", req.Name,
	)

	var heartbeat monitoringv1alpha1.Heartbeat
	if err := r.Get(ctx, req.NamespacedName, &heartbeat); err != nil {
		if apierrors.IsNotFound(err) {
			l.V(1).Info("Heartbeat resource not found",
				"namespace", req.Namespace,
				"name", req.Name,
			)
			return nil, nil // Resource was deleted
		}
		l.Error(err, "Failed to get heartbeat resource",
			"namespace", req.Namespace,
			"name", req.Name,
		)
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
	l logr.Logger,
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

	l.V(2).Info("Fetching secret",
		"namespace", heartbeat.Namespace,
		"name", heartbeat.Name,
		"secret_namespace", secretNamespace,
		"secret_name", heartbeat.Spec.EndpointsSecret.Name,
	)

	if getErr := r.Get(ctx, secretKey, secret); getErr != nil {
		l.Error(getErr, "Failed to fetch secret",
			"namespace", heartbeat.Namespace,
			"name", heartbeat.Name,
			"secret_namespace", secretNamespace,
			"secret_name", heartbeat.Spec.EndpointsSecret.Name,
		)
		if updateErr := r.StatusUpdater.UpdateSecretErrorStatus(ctx, heartbeat, getErr); updateErr != nil {
			l.Error(updateErr, "Failed to update secret error status",
				"namespace", heartbeat.Namespace,
				"name", heartbeat.Name,
			)
			return nil, updateErr
		}
		return nil, getErr
	}

	l.V(2).Info("Secret fetched successfully",
		"namespace", heartbeat.Namespace,
		"name", heartbeat.Name,
		"secret_namespace", secretNamespace,
		"secret_name", heartbeat.Spec.EndpointsSecret.Name,
	)

	return secret, nil
}

// extractTargetEndpoint extracts and validates the target endpoint URL from the secret.
// Returns an error if the target endpoint key is missing or empty, which will trigger status update and requeue.
func (r *HeartbeatReconciler) extractTargetEndpoint(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	secret *corev1.Secret,
	l logr.Logger,
) (string, error) {
	l.V(2).Info("Extracting target endpoint",
		"namespace", heartbeat.Namespace,
		"name", heartbeat.Name,
		"target_endpoint_key", heartbeat.Spec.EndpointsSecret.TargetEndpointKey,
	)

	endpoint, err := r.extractEndpointFromSecret(
		ctx, heartbeat, secret, heartbeat.Spec.EndpointsSecret.TargetEndpointKey, l,
	)
	if err != nil {
		l.Error(err, "Failed to extract target endpoint from secret",
			"namespace", heartbeat.Namespace,
			"name", heartbeat.Name,
			"target_endpoint_key", heartbeat.Spec.EndpointsSecret.TargetEndpointKey,
		)
		return "", err
	}

	if endpoint == "" {
		l.Error(fmt.Errorf("target endpoint is empty"), "Target endpoint is empty",
			"namespace", heartbeat.Namespace,
			"name", heartbeat.Name,
			"target_endpoint_key", heartbeat.Spec.EndpointsSecret.TargetEndpointKey,
		)
		if updateErr := r.StatusUpdater.UpdateEmptyEndpointStatus(ctx, heartbeat); updateErr != nil {
			l.Error(updateErr, "Failed to update empty endpoint status",
				"namespace", heartbeat.Namespace,
				"name", heartbeat.Name,
			)
			return "", updateErr
		}
		return "", fmt.Errorf("target endpoint is empty")
	}

	l.V(2).Info("Target endpoint extracted successfully",
		"namespace", heartbeat.Namespace,
		"name", heartbeat.Name,
		"target_endpoint", endpoint,
	)

	return endpoint, nil
}

// extractEndpointFromSecret extracts an endpoint URL from the secret by key.
// Returns an error if the key is missing, which will trigger status update.
func (r *HeartbeatReconciler) extractEndpointFromSecret(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	secret *corev1.Secret,
	key string,
	l logr.Logger,
) (string, error) {
	l.V(3).Info("Extracting endpoint from secret",
		"namespace", heartbeat.Namespace,
		"name", heartbeat.Name,
		"key", key,
	)

	endpointBytes, ok := secret.Data[key]
	if !ok {
		l.Error(fmt.Errorf("missing endpoint key: %s", key), "Missing endpoint key in secret",
			"namespace", heartbeat.Namespace,
			"name", heartbeat.Name,
			"key", key,
			"available_keys", getSecretKeys(secret),
		)
		if updateErr := r.StatusUpdater.UpdateMissingKeyStatus(ctx, heartbeat, key); updateErr != nil {
			l.Error(updateErr, "Failed to update missing key status",
				"namespace", heartbeat.Namespace,
				"name", heartbeat.Name,
				"key", key,
			)
			return "", updateErr
		}
		return "", fmt.Errorf("missing endpoint key: %s", key)
	}

	endpoint := string(endpointBytes)
	l.V(3).Info("Endpoint extracted from secret",
		"namespace", heartbeat.Namespace,
		"name", heartbeat.Name,
		"key", key,
		"endpoint", endpoint,
	)

	return endpoint, nil
}

// extractReportEndpoints extracts the healthy and unhealthy endpoint URLs from the secret for reporting.
// Returns an error if either report endpoint key is missing, which will trigger status update and requeue.
func (r *HeartbeatReconciler) extractReportEndpoints(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	secret *corev1.Secret,
) (monitoringv1alpha1.EndpointsSecret, error) {
	loggerInstance := log.FromContext(ctx).WithName(logNameHeartbeatReconciler)

	loggerInstance.V(2).Info("Extracting report endpoints",
		"namespace", heartbeat.Namespace,
		"name", heartbeat.Name,
		"healthy_endpoint_key", heartbeat.Spec.EndpointsSecret.HealthyEndpointKey,
		"unhealthy_endpoint_key", heartbeat.Spec.EndpointsSecret.UnhealthyEndpointKey,
	)

	// Extract both endpoints using helper function
	healthyEndpoint, err := r.extractSingleReportEndpoint(
		ctx, heartbeat, secret, heartbeat.Spec.EndpointsSecret.HealthyEndpointKey, "healthy", loggerInstance,
	)
	if err != nil {
		return monitoringv1alpha1.EndpointsSecret{}, err
	}

	unhealthyEndpoint, err := r.extractSingleReportEndpoint(
		ctx, heartbeat, secret, heartbeat.Spec.EndpointsSecret.UnhealthyEndpointKey, "unhealthy", loggerInstance,
	)
	if err != nil {
		return monitoringv1alpha1.EndpointsSecret{}, err
	}

	// Create EndpointsSecret with actual URLs for reporting
	reportEndpointsSecret := monitoringv1alpha1.EndpointsSecret{
		HealthyEndpointKey:   healthyEndpoint,
		UnhealthyEndpointKey: unhealthyEndpoint,
	}

	loggerInstance.V(2).Info("Report endpoints extracted successfully",
		"namespace", heartbeat.Namespace,
		"name", heartbeat.Name,
		"healthy_endpoint", healthyEndpoint,
		"unhealthy_endpoint", unhealthyEndpoint,
	)

	return reportEndpointsSecret, nil
}

// extractSingleReportEndpoint extracts a single report endpoint from the secret.
func (r *HeartbeatReconciler) extractSingleReportEndpoint(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	secret *corev1.Secret,
	key, endpointType string,
	loggerInstance logr.Logger,
) (string, error) {
	endpoint, err := r.extractEndpointFromSecret(ctx, heartbeat, secret, key, loggerInstance)
	if err != nil {
		return "", logger.LogEndpointExtractionError(
			loggerInstance,
			"secret",
			heartbeat.Namespace,
			heartbeat.Name,
			endpointType,
			key,
			err,
		)
	}
	return endpoint, nil
}

// performHealthCheckAndReport performs the actual health check and reports the status.
// This is the core business logic that determines if the endpoint is healthy and reports accordingly.
func (r *HeartbeatReconciler) performHealthCheckAndReport(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	targetEndpoint string,
	reportEndpointsSecret monitoringv1alpha1.EndpointsSecret,
) error {
	l := log.FromContext(ctx).WithName(logNameHeartbeatReconciler)

	l.V(1).Info("Starting health check and report",
		"namespace", heartbeat.Namespace,
		"name", heartbeat.Name,
		"target_endpoint", targetEndpoint,
		"expected_status_ranges", heartbeat.Spec.ExpectedStatusCodeRanges,
	)

	// Validate status code ranges before health check
	if r.performHealthCheckValidation(ctx, heartbeat) {
		return nil // Early return, status already updated
	}

	// Perform the health check
	healthy, statusCode, reportSuccess, err := r.performHealthCheckExecution(
		ctx, heartbeat, targetEndpoint, reportEndpointsSecret,
	)
	if err != nil {
		return r.handleHealthCheckError(ctx, heartbeat, statusCode, err)
	}

	// Update status with successful health check results
	return r.performHealthCheckStatusUpdate(ctx, heartbeat, healthy, statusCode, reportSuccess)
}

// performHealthCheckValidation validates status code ranges before performing health check.
func (r *HeartbeatReconciler) performHealthCheckValidation(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
) bool {
	l := log.FromContext(ctx).WithName(logNameHeartbeatReconciler)

	if r.validateStatusCodeRanges(ctx, heartbeat) {
		l.V(1).Info("Invalid status code ranges detected, skipping health check",
			"namespace", heartbeat.Namespace,
			"name", heartbeat.Name,
		)
		return true // Invalid ranges found
	}
	return false // Valid ranges
}

// performHealthCheckExecution executes the actual health check and handles the response.
func (r *HeartbeatReconciler) performHealthCheckExecution(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	targetEndpoint string,
	reportEndpointsSecret monitoringv1alpha1.EndpointsSecret,
) (bool, int, bool, error) {
	l := log.FromContext(ctx).WithName(logNameHeartbeatReconciler)

	// Perform the health check
	healthy, statusCode, reportSuccess, err := r.HealthChecker.CheckEndpointHealth(
		ctx,
		targetEndpoint,
		heartbeat.Spec.ExpectedStatusCodeRanges,
		reportEndpointsSecret,
	)

	if err != nil {
		l.Error(err, "Health check failed",
			"namespace", heartbeat.Namespace,
			"name", heartbeat.Name,
			"target_endpoint", targetEndpoint,
			"status_code", statusCode,
		)
		return false, statusCode, false, err
	}

	l.Info("Health check completed",
		"namespace", heartbeat.Namespace,
		"name", heartbeat.Name,
		"target_endpoint", targetEndpoint,
		"status_code", statusCode,
		"healthy", healthy,
		"report_success", reportSuccess,
	)

	return healthy, statusCode, reportSuccess, nil
}

// performHealthCheckStatusUpdate updates the status with health check results.
func (r *HeartbeatReconciler) performHealthCheckStatusUpdate(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	healthy bool,
	statusCode int,
	reportSuccess bool,
) error {
	l := log.FromContext(ctx).WithName(logNameHeartbeatReconciler)

	if err := r.updateHealthStatus(ctx, heartbeat, healthy, statusCode, reportSuccess); err != nil {
		l.Error(err, "Failed to update health status",
			"namespace", heartbeat.Namespace,
			"name", heartbeat.Name,
		)
		return err
	}

	return nil
}

// handleHealthCheckError handles errors during health checks by updating status and returning the error.
func (r *HeartbeatReconciler) handleHealthCheckError(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
	statusCode int,
	err error,
) error {
	l := log.FromContext(ctx).WithName(logNameHeartbeatReconciler)

	l.Error(err, "Handling health check error",
		"namespace", heartbeat.Namespace,
		"name", heartbeat.Name,
		"status_code", statusCode,
	)

	if updateErr := r.StatusUpdater.UpdateHealthCheckErrorStatus(ctx, heartbeat, statusCode, err); updateErr != nil {
		l.Error(updateErr, "Failed to update status after health check error",
			"namespace", heartbeat.Namespace,
			"name", heartbeat.Name,
		)
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
	l := log.FromContext(ctx).WithName(logNameHeartbeatReconciler)

	l.V(1).Info("Updating health status",
		"namespace", heartbeat.Namespace,
		"name", heartbeat.Name,
		"healthy", healthy,
		"status_code", statusCode,
		"report_success", reportSuccess,
	)

	if err := r.StatusUpdater.UpdateHealthStatus(ctx, heartbeat, healthy, statusCode, nil, reportSuccess); err != nil {
		l.Error(err, "Failed to update health status",
			"namespace", heartbeat.Namespace,
			"name", heartbeat.Name,
		)
		return fmt.Errorf("failed to update health status: %w", err)
	}

	l.V(1).Info("Health status updated successfully",
		"namespace", heartbeat.Namespace,
		"name", heartbeat.Name,
		"healthy", healthy,
		"status_code", statusCode,
	)

	return nil
}

// validateStatusCodeRanges validates that all status code ranges have valid min/max values.
// Returns true if an invalid range was found and status was updated.
func (r *HeartbeatReconciler) validateStatusCodeRanges(
	ctx context.Context,
	heartbeat *monitoringv1alpha1.Heartbeat,
) bool {
	l := log.FromContext(ctx).WithName(logNameHeartbeatReconciler)

	l.V(2).Info("Validating status code ranges",
		"namespace", heartbeat.Namespace,
		"name", heartbeat.Name,
		"status_ranges", heartbeat.Spec.ExpectedStatusCodeRanges,
	)

	for i, statusRange := range heartbeat.Spec.ExpectedStatusCodeRanges {
		if statusRange.Min > statusRange.Max {
			msg := fmt.Errorf("invalid status code range: min %d > max %d", statusRange.Min, statusRange.Max)
			l.Error(msg, "Invalid status code range",
				"namespace", heartbeat.Namespace,
				"name", heartbeat.Name,
				"range_index", i,
				"min", statusRange.Min,
				"max", statusRange.Max,
			)

			if updateErr := r.StatusUpdater.UpdateInvalidRangeStatus(
				ctx,
				heartbeat,
				0, // No status code available before health check
			); updateErr != nil {
				// Log error but don't fail the reconciliation
				l.Error(updateErr, "Failed to update invalid range status",
					"namespace", heartbeat.Namespace,
					"name", heartbeat.Name,
				)
			}
			return true // Invalid range found
		}
	}

	l.V(2).Info("Status code ranges validation passed",
		"namespace", heartbeat.Namespace,
		"name", heartbeat.Name,
	)
	return false // No invalid ranges found
}

// getSecretKeys returns a slice of secret keys for logging purposes
func getSecretKeys(secret *corev1.Secret) []string {
	keys := make([]string, 0, len(secret.Data))
	for key := range secret.Data {
		keys = append(keys, key)
	}
	return keys
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
		WithOptions(controller.Options{
			MaxConcurrentReconciles: MaxConcurrentReconciles,
		}).
		Complete(r)
}
