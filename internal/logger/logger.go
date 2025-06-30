package logger

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// WithRequest creates a logger with request context (namespace and name)
func WithRequest(ctx context.Context, component string, namespace, name string) logr.Logger {
	return log.FromContext(ctx).WithName(component).WithValues(
		"namespace", namespace,
		"name", name,
	)
}

// WithHeartbeat creates a logger with heartbeat context
func WithHeartbeat(ctx context.Context, component string, namespace, name string) logr.Logger {
	return log.FromContext(ctx).WithName(component).WithValues(
		"namespace", namespace,
		"name", name,
	)
}

// Info logs an informational message with optional additional fields.
func Info(log logr.Logger, message string, additionalFields map[string]interface{}) {
	if additionalFields != nil {
		log.V(1).Info(message, additionalFields)
	} else {
		log.V(1).Info(message)
	}
}

// Error logs an error message with optional additional fields.
func Error(log logr.Logger, message string, err error, additionalFields map[string]interface{}) {
	if additionalFields != nil {
		log.Error(err, message, additionalFields)
	} else {
		log.Error(err, message)
	}
}

// LogEndpointExtractionError logs an error when extracting an endpoint from a secret or similar resource.
func LogEndpointExtractionError(
	log logr.Logger,
	resourceType, resourceNamespace, resourceName, endpointType, key string,
	err error,
) error {
	log.Error(err, "Failed to extract "+endpointType+" endpoint from "+resourceType,
		"namespace", resourceNamespace,
		"name", resourceName,
		endpointType+"_endpoint_key", key,
	)
	return err
}
