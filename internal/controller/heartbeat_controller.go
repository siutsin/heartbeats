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
	log := log.FromContext(ctx)

	var heartbeat monitoringv1alpha1.Heartbeat
	if err := r.Get(ctx, req.NamespacedName, &heartbeat); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, ErrFailedToGetSecret)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Get the endpoint from the secret
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
		if err := r.StatusUpdater.UpdateSecretErrorStatus(ctx, &heartbeat, err); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: r.Config.RequeueAfter}, nil
	}

	endpointBytes, ok := secret.Data[heartbeat.Spec.EndpointsSecret.TargetEndpointKey]
	if !ok {
		if err := r.StatusUpdater.UpdateMissingKeyStatus(
			ctx,
			&heartbeat,
			heartbeat.Spec.EndpointsSecret.TargetEndpointKey,
		); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: r.Config.RequeueAfter}, nil
	}

	endpoint := string(endpointBytes)
	if endpoint == "" {
		if err := r.StatusUpdater.UpdateEmptyEndpointStatus(ctx, &heartbeat); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: r.Config.RequeueAfter}, nil
	}

	// Check the endpoint health
	healthy, statusCode, err := r.HealthChecker.CheckEndpointHealth(
		ctx,
		endpoint,
		heartbeat.Spec.ExpectedStatusCodeRanges,
	)
	if err != nil {
		if err := r.StatusUpdater.UpdateHealthCheckErrorStatus(
			ctx,
			&heartbeat,
			statusCode,
			err,
		); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: r.Config.RequeueAfter}, nil
	}

	// Check if any of the status code ranges are invalid
	for _, statusRange := range heartbeat.Spec.ExpectedStatusCodeRanges {
		if statusRange.Min > statusRange.Max {
			if err := r.StatusUpdater.UpdateInvalidRangeStatus(
				ctx,
				&heartbeat,
				statusCode,
			); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: r.Config.RequeueAfter}, nil
		}
	}

	// Check if the status code is within any of the expected ranges
	if isStatusCodeInRange(statusCode, heartbeat.Spec.ExpectedStatusCodeRanges) {
		healthy = true
	}

	if err := r.StatusUpdater.UpdateHealthStatus(
		ctx,
		&heartbeat,
		healthy,
		statusCode,
		nil,
	); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.Config.RequeueAfter}, nil
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
