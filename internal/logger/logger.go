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
	logger      *slog.Logger
	minLogLevel int // minimum enabled log level, e.g., 0 for info, -1 for debug
}

func (s *slogLogger) Init(_ logr.RuntimeInfo) {}

func (s *slogLogger) Enabled(level int) bool {
	// Returns true if the log level is enabled.
	// minLogLevel is configurable; by default, info (0) and above are enabled.
	return level <= s.minLogLevel
}

func (s *slogLogger) Info(_ int, msg string, keysAndValues ...interface{}) {
	attrs := make([]any, 0, len(keysAndValues))
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			attrs = append(attrs, keysAndValues[i], keysAndValues[i+1])
		} else {
			// Odd number of elements: add a placeholder value for the last key
			attrs = append(attrs, keysAndValues[i], "(MISSING)")
		}
	}
	s.logger.Info(msg, attrs...)
}

func (s *slogLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	attrs := make([]any, 0, len(keysAndValues)+2)

	// Check if "error" key already exists in keysAndValues to avoid collision
	hasErrorKey := false
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) && keysAndValues[i] == "error" {
			hasErrorKey = true
			break
		}
	}

	// Only prepend error if it's not already present
	if !hasErrorKey {
		attrs = append(attrs, "error", err)
	}

	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			attrs = append(attrs, keysAndValues[i], keysAndValues[i+1])
		} else {
			// Odd number of elements: add a placeholder value for the last key
			attrs = append(attrs, keysAndValues[i], "(MISSING)")
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
			// Odd number of elements: add a placeholder value for the last key
			attrs = append(attrs, keysAndValues[i], "(MISSING)")
		}
	}
	return &slogLogger{
		logger:      s.logger.With(attrs...),
		minLogLevel: s.minLogLevel,
	}
}

func (s *slogLogger) WithName(name string) logr.LogSink {
	return &slogLogger{
		logger:      s.logger.With("name", name),
		minLogLevel: s.minLogLevel,
	}
}

// NewSlogLogger returns a logr.Logger backed by the provided slog.Logger
// minLogLevel follows logr convention: 0=info, -1=debug, etc.
func NewSlogLogger(l *slog.Logger) logr.Logger {
	return logr.New(&slogLogger{
		logger:      l,
		minLogLevel: 0, // Default to info level and above
	})
}

// NewSlogLoggerWithLevel returns a logr.Logger backed by the provided slog.Logger with custom log level
// minLogLevel follows logr convention: 0=info, -1=debug, etc.
func NewSlogLoggerWithLevel(l *slog.Logger, minLogLevel int) logr.Logger {
	return logr.New(&slogLogger{
		logger:      l,
		minLogLevel: minLogLevel,
	})
}
