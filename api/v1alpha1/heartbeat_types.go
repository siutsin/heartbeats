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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EndpointsSecret defines the configuration for the secret containing endpoint URLs
type EndpointsSecret struct {
	// Name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the secret. If empty, defaults to the same namespace as the Heartbeat resource
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// TargetEndpointKey is the key in the secret that contains the target endpoint URL
	// +kubebuilder:validation:Required
	TargetEndpointKey string `json:"targetEndpointKey"`

	// HealthyEndpointKey is the key in the secret that contains the healthy endpoint URL
	// +kubebuilder:validation:Required
	HealthyEndpointKey string `json:"healthyEndpointKey"`

	// UnhealthyEndpointKey is the key in the secret that contains the unhealthy endpoint URL
	// +kubebuilder:validation:Required
	UnhealthyEndpointKey string `json:"unhealthyEndpointKey"`

	// HealthyEndpointMethod is the HTTP method to use when reporting to the healthy endpoint
	// +kubebuilder:validation:Enum=GET;POST;PUT;PATCH
	// +kubebuilder:default=GET
	// +optional
	HealthyEndpointMethod string `json:"healthyEndpointMethod,omitempty"`

	// UnhealthyEndpointMethod is the HTTP method to use when reporting to the unhealthy endpoint
	// +kubebuilder:validation:Enum=GET;POST;PUT;PATCH
	// +kubebuilder:default=GET
	// +optional
	UnhealthyEndpointMethod string `json:"unhealthyEndpointMethod,omitempty"`
}

// StatusCodeRange defines a range of HTTP status codes
type StatusCodeRange struct {
	// Min is the minimum status code in the range (inclusive)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:Maximum=599
	Min int `json:"min"`

	// Max is the maximum status code in the range (inclusive)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:Maximum=599
	Max int `json:"max"`
}

// HeartbeatSpec defines the desired state of Heartbeat.
type HeartbeatSpec struct {
	// EndpointsSecret is the reference to the secret containing all endpoint URLs
	// +kubebuilder:validation:Required
	EndpointsSecret EndpointsSecret `json:"endpointsSecret"`

	// ExpectedStatusCodeRanges defines the ranges of HTTP status codes that are considered healthy
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	ExpectedStatusCodeRanges []StatusCodeRange `json:"expectedStatusCodeRanges"`

	// Interval is the time between health checks
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=^([0-9]+(s|m|h))$
	// +kubebuilder:validation:Description="Duration between health checks (e.g., 30s, 5m, 1h)"
	// +kubebuilder:default="60s"
	Interval string `json:"interval"`
}

// HeartbeatStatus defines the observed state of Heartbeat.
type HeartbeatStatus struct {
	// Healthy indicates whether the endpoint is healthy
	Healthy bool `json:"healthy"`

	// LastStatus contains the last HTTP status code received from the endpoint
	LastStatus int `json:"lastStatus"`

	// Message contains a human-readable message about the endpoint status
	Message string `json:"message"`

	// LastChecked is the timestamp of the last health check
	LastChecked *metav1.Time `json:"lastChecked,omitempty"`

	// ReportStatus indicates if the last report (to healthy/unhealthy endpoint) was successful
	ReportStatus string `json:"reportStatus,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Heartbeat is the Schema for the heartbeats API.
type Heartbeat struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HeartbeatSpec   `json:"spec,omitempty"`
	Status HeartbeatStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HeartbeatList contains a list of Heartbeat.
type HeartbeatList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Heartbeat `json:"items"`
}
