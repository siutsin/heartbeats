package logger_test

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/onsi/gomega"

	"github.com/siutsin/heartbeats/internal/logger"
)

// testLogSink is a simple implementation that captures log calls
type testLogSink struct {
	infoCalls  []infoCall
	errorCalls []errorCall
}

type infoCall struct {
	level int
	msg   string
	keys  []interface{}
}

type errorCall struct {
	err  error
	msg  string
	keys []interface{}
}

func (t *testLogSink) Init(_ logr.RuntimeInfo) {}
func (t *testLogSink) Enabled(_ int) bool      { return true }
func (t *testLogSink) Info(level int, msg string, keysAndValues ...interface{}) {
	t.infoCalls = append(t.infoCalls, infoCall{level: level, msg: msg, keys: keysAndValues})
}
func (t *testLogSink) Error(err error, msg string, keysAndValues ...interface{}) {
	t.errorCalls = append(t.errorCalls, errorCall{err: err, msg: msg, keys: keysAndValues})
}
func (t *testLogSink) WithValues(_ ...interface{}) logr.LogSink { return t }
func (t *testLogSink) WithName(_ string) logr.LogSink           { return t }

func TestWithRequest(t *testing.T) {
	g := gomega.NewWithT(t)

	ctx := context.Background()
	log := logger.WithRequest(ctx, "test-component", "test-namespace", "test-name")
	g.Expect(log).NotTo(gomega.BeNil())
}

func TestWithHeartbeat(t *testing.T) {
	g := gomega.NewWithT(t)

	ctx := context.Background()
	log := logger.WithHeartbeat(ctx, "test-component", "test-namespace", "test-name")
	g.Expect(log).NotTo(gomega.BeNil())
}

func TestInfo(t *testing.T) {
	tests := []struct {
		name             string
		message          string
		additionalFields map[string]interface{}
		description      string
	}{
		{
			name:             "with nil additional fields",
			message:          "test message",
			additionalFields: nil,
			description:      "should log without additional fields",
		},
		{
			name:    "with additional fields",
			message: "test message with fields",
			additionalFields: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			description: "should log with additional fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			// Create a real log sink that captures calls
			sink := &testLogSink{}
			log := logr.New(sink)

			// Call the function
			logger.Info(log, tt.message, tt.additionalFields)

			// Verify the call was made
			g.Expect(sink.infoCalls).To(gomega.HaveLen(1))
			g.Expect(sink.infoCalls[0].level).To(gomega.Equal(1))
			g.Expect(sink.infoCalls[0].msg).To(gomega.Equal(tt.message))

			if tt.additionalFields != nil {
				g.Expect(sink.infoCalls[0].keys).To(gomega.ContainElement(tt.additionalFields))
			}
		})
	}
}

func TestError(t *testing.T) {
	tests := []struct {
		name             string
		message          string
		err              error
		additionalFields map[string]interface{}
		description      string
	}{
		{
			name:             "with nil additional fields",
			message:          "test error message",
			err:              errors.New("test error"),
			additionalFields: nil,
			description:      "should log error without additional fields",
		},
		{
			name:    "with additional fields",
			message: "test error message with fields",
			err:     errors.New("test error"),
			additionalFields: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			description: "should log error with additional fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			// Create a real log sink that captures calls
			sink := &testLogSink{}
			log := logr.New(sink)

			// Call the function
			logger.Error(log, tt.message, tt.err, tt.additionalFields)

			// Verify the call was made
			g.Expect(sink.errorCalls).To(gomega.HaveLen(1))
			g.Expect(sink.errorCalls[0].err).To(gomega.Equal(tt.err))
			g.Expect(sink.errorCalls[0].msg).To(gomega.Equal(tt.message))

			if tt.additionalFields != nil {
				g.Expect(sink.errorCalls[0].keys).To(gomega.ContainElement(tt.additionalFields))
			}
		})
	}
}

func TestLogEndpointExtractionError(t *testing.T) {
	tests := []struct {
		name              string
		resourceType      string
		resourceNamespace string
		resourceName      string
		endpointType      string
		key               string
		err               error
		expectedKeys      []interface{}
		description       string
	}{
		{
			name:              "secret extraction error",
			resourceType:      "secret",
			resourceNamespace: "test-namespace",
			resourceName:      "test-name",
			endpointType:      "healthy",
			key:               "test-key",
			err:               errors.New("test extraction error"),
			expectedKeys: []interface{}{
				"namespace", "test-namespace", "name", "test-name",
				"healthy_endpoint_key", "test-key",
			},
			description: "should log secret extraction error with correct fields",
		},
		{
			name:              "configmap extraction error",
			resourceType:      "configmap",
			resourceNamespace: "other-namespace",
			resourceName:      "other-name",
			endpointType:      "unhealthy",
			key:               "other-key",
			err:               errors.New("test extraction error"),
			expectedKeys: []interface{}{
				"namespace", "other-namespace", "name", "other-name",
				"unhealthy_endpoint_key", "other-key",
			},
			description: "should log configmap extraction error with correct fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			// Create a real log sink that captures calls
			sink := &testLogSink{}
			log := logr.New(sink)

			// Call the function
			returnedErr := logger.LogEndpointExtractionError(
				log,
				tt.resourceType,
				tt.resourceNamespace,
				tt.resourceName,
				tt.endpointType,
				tt.key,
				tt.err,
			)

			// Verify the returned error is the same
			g.Expect(returnedErr).To(gomega.Equal(tt.err))

			// Verify the error was logged
			g.Expect(sink.errorCalls).To(gomega.HaveLen(1))
			g.Expect(sink.errorCalls[0].err).To(gomega.Equal(tt.err))

			expectedMessage := "Failed to extract " + tt.endpointType + " endpoint from " + tt.resourceType
			g.Expect(sink.errorCalls[0].msg).To(gomega.Equal(expectedMessage))

			// Verify the keys were logged
			for _, key := range tt.expectedKeys {
				g.Expect(sink.errorCalls[0].keys).To(gomega.ContainElement(key))
			}
		})
	}
}
