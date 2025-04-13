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
	"fmt"
	"regexp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// EndpointSecretRef defines a reference to a Kubernetes secret containing endpoint information
type EndpointSecretRef struct {
	// Name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the secret. If empty, defaults to the same namespace as the Heartbeat resource
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Key in the secret that contains the endpoint URL
	// +kubebuilder:validation:Required
	Key string `json:"key"`
}

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
}

// String returns a human-readable representation of the status
func (s HeartbeatStatus) String() string {
	status := "Unhealthy"
	if s.Healthy {
		status = "Healthy"
	}
	return fmt.Sprintf("%s (%d): %s", status, s.LastStatus, s.Message)
}

// ValidateCreate validates the Heartbeat resource on creation
func (h *Heartbeat) ValidateCreate() error {
	if h.Spec.EndpointsSecret.Name == "" {
		return fmt.Errorf("endpointsSecret.name is required")
	}
	if h.Spec.EndpointsSecret.TargetEndpointKey == "" {
		return fmt.Errorf("endpointsSecret.targetEndpointKey is required")
	}
	if h.Spec.EndpointsSecret.HealthyEndpointKey == "" {
		return fmt.Errorf("endpointsSecret.healthyEndpointKey is required")
	}
	if h.Spec.EndpointsSecret.UnhealthyEndpointKey == "" {
		return fmt.Errorf("endpointsSecret.unhealthyEndpointKey is required")
	}
	if len(h.Spec.ExpectedStatusCodeRanges) == 0 {
		return fmt.Errorf("expectedStatusCodeRanges must contain at least one range")
	}
	for _, r := range h.Spec.ExpectedStatusCodeRanges {
		if r.Min < 100 || r.Min > 599 {
			return fmt.Errorf("status code range min must be between 100 and 599")
		}
		if r.Max < 100 || r.Max > 599 {
			return fmt.Errorf("status code range max must be between 100 and 599")
		}
		if r.Min > r.Max {
			return fmt.Errorf("status code range min must be less than or equal to max")
		}
	}
	if h.Spec.Interval == "" {
		return fmt.Errorf("interval is required")
	}
	if !regexp.MustCompile(`^([0-9]+(s|m|h))$`).MatchString(h.Spec.Interval) {
		return fmt.Errorf("interval must match pattern ^([0-9]+(s|m|h))$")
	}
	return nil
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

func init() {
	SchemeBuilder.Register(&Heartbeat{}, &HeartbeatList{})
}
