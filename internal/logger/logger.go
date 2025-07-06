package logger

import (
	"context"

	"log/slog"

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

// slogLogger implements logr.Logger interface using slog
// This allows integration of Go's slog with logr consumers (e.g., controller-runtime)
type slogLogger struct {
	logger *slog.Logger
}

func (s *slogLogger) Init(_ logr.RuntimeInfo) {}

func (s *slogLogger) Enabled(level int) bool {
	return level <= 0 // info and above, debug levels disabled for now
}

func (s *slogLogger) Info(_ int, msg string, keysAndValues ...interface{}) {
	attrs := make([]any, 0, len(keysAndValues))
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			attrs = append(attrs, keysAndValues[i], keysAndValues[i+1])
		} else {
			attrs = append(attrs, keysAndValues[i])
		}
	}
	s.logger.Info(msg, attrs...)
}

func (s *slogLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	attrs := make([]any, 0, len(keysAndValues)+2)
	attrs = append(attrs, "error", err)
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			attrs = append(attrs, keysAndValues[i], keysAndValues[i+1])
		} else {
			attrs = append(attrs, keysAndValues[i])
		}
	}
	s.logger.Error(msg, attrs...)
}

func (s *slogLogger) WithValues(keysAndValues ...interface{}) logr.LogSink {
	attrs := make([]any, 0, len(keysAndValues))
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			attrs = append(attrs, keysAndValues[i], keysAndValues[i+1])
		} else {
			attrs = append(attrs, keysAndValues[i])
		}
	}
	return &slogLogger{
		logger: s.logger.With(attrs...),
	}
}

func (s *slogLogger) WithName(name string) logr.LogSink {
	return &slogLogger{
		logger: s.logger.With("logger", name),
	}
}

// NewSlogLogger returns a logr.Logger backed by the provided slog.Logger
func NewSlogLogger(l *slog.Logger) logr.Logger {
	return logr.New(&slogLogger{logger: l})
}
