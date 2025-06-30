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

package controller_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	monitoringv1alpha1 "github.com/siutsin/heartbeats/api/v1alpha1"
	"github.com/siutsin/heartbeats/internal/controller"

	// +kubebuilder:scaffold:imports
	"log/slog"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

// Global variables used across the test suite for shared resources.
var (
	cfg       *rest.Config         // Kubernetes REST config for the test environment
	k8sClient client.Client        // Kubernetes client for interacting with the test cluster
	testEnv   *envtest.Environment // Test environment providing a local Kubernetes cluster
	ctx       context.Context      // Context for test operations
	cancel    context.CancelFunc   // Cancel function for the test context
)

// TestAPIs is the main entry point for the test suite.
// It registers the Ginkgo fail handler and runs all test specs.
// This function is called by the Go testing framework.
func TestAPIs(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)

	ginkgo.RunSpecs(t, "Controller Suite")
}

// BeforeSuite sets up the test environment before any tests run.
// It initialises a local Kubernetes cluster, creates the necessary clients,
// and starts the controller manager for integration testing.
var _ = ginkgo.BeforeSuite(func() {
	// Configure slog for tests
	handler := slog.NewJSONHandler(ginkgo.GinkgoWriter, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Set up zap as the logr implementation
	logf.SetLogger(zap.New(zap.WriteTo(ginkgo.GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)

	ginkgo.By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:       []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing:   true,
		ControlPlaneStopTimeout: 9 * time.Second,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(cfg).NotTo(gomega.BeNil())

	err = monitoringv1alpha1.AddToScheme(scheme.Scheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(k8sClient).NotTo(gomega.BeNil())

	// Initialize the global K8sClient
	controller.K8sClient = k8sClient

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	err = (&controller.HeartbeatReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	go func() {
		defer ginkgo.GinkgoRecover()
		err = k8sManager.Start(ctx)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}()
})

// AfterSuite cleans up the test environment after all tests have completed.
// It cancels the test context and stops the local Kubernetes cluster.
var _ = ginkgo.AfterSuite(func() {
	ginkgo.By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
})
