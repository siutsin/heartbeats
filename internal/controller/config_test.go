package controller_test

import (
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/siutsin/heartbeats/internal/controller"
)

func TestDefaultConfig(t *testing.T) {
	g := gomega.NewWithT(t)

	config := controller.DefaultConfig()

	g.Expect(config.DefaultTimeout).To(gomega.Equal(10 * time.Second))
	g.Expect(config.MaxRetries).To(gomega.Equal(3))
	g.Expect(config.RetryDelay).To(gomega.Equal(1 * time.Second))
	g.Expect(config.RequeueAfter).To(gomega.Equal(5 * time.Second))
}
