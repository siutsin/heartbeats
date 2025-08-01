package controller_test

import (
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/siutsin/heartbeats/internal/controller"
)

// TestDefaultConfig verifies that the default configuration returns the expected values.
// This test ensures that the default configuration provides sensible defaults for production use.
// It checks that all configuration fields have the correct default values.
func TestDefaultConfig(t *testing.T) {
	g := gomega.NewWithT(t)

	config := controller.DefaultConfig()

	g.Expect(config.DefaultTimeout).To(gomega.Equal(10 * time.Second))
	g.Expect(config.MaxRetries).To(gomega.Equal(3))
	g.Expect(config.RetryDelay).To(gomega.Equal(1 * time.Second))
	g.Expect(config.RequeueAfter).To(gomega.Equal(5 * time.Second))
}
