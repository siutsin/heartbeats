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

// Package e2e contains the test suite configuration for end-to-end tests.
// This file sets up the test environment, including creating a Kind cluster,
// building and deploying the operator, and installing required dependencies.
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/siutsin/heartbeats/test/utils"
)

// projectImage is the name of the Docker image used for testing the operator.
// It defaults to heartbeats-operator:test and can be overridden with E2E_IMG
// so the Makefile, CI workflow, and test suite share a single tag.
var projectImage = e2eImage()

// e2eImage returns the operator image tag for e2e tests.
func e2eImage() string {
	if v, ok := os.LookupEnv("E2E_IMG"); ok && v != "" {
		return v
	}
	return "heartbeats-operator:test"
}

// TestE2E is the main test function that runs the e2e test suite.
// It registers the gomega fail handler with ginkgo and runs all test specs.
//
// Parameters:
//   - t: The testing.T instance for the test
func TestE2E(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "E2E Suite")
}

// BeforeSuite sets up the complete test environment before any tests run.
// This includes creating a Kind cluster, building the operator image,
// deploying the operator, and installing required dependencies.
var _ = ginkgo.BeforeSuite(func() {
	ginkgo.By("Setting up test environment")

	setupKindCluster()
	buildAndLoadOperatorImage()
	deployOperator()
})

// AfterSuite cleans up the test environment after all tests complete.
// It removes the Kind cluster to free up system resources.
var _ = ginkgo.AfterSuite(func() {
	ginkgo.By("Tearing down test environment")
	if os.Getenv("CLUSTER_BACKEND") == "apple" {
		ginkgo.By("leaving the Apple Container cluster running")
		return
	}
	ginkgo.By("deleting Kind cluster")
	cmd := exec.Command("kind", "delete", "cluster")
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to delete Kind cluster")
})

// setupKindCluster creates a fresh Kind cluster for testing.
// It first deletes any existing cluster to ensure a clean environment,
// then creates a new cluster using the configuration file.
func setupKindCluster() {
	if os.Getenv("CLUSTER_BACKEND") == "apple" {
		ginkgo.By("using the Apple Container cluster prepared by the Makefile")
		return
	}
	ginkgo.By("deleting any existing Kind cluster")
	cmd := exec.Command("kind", "delete", "cluster")
	_, err := utils.Run(cmd)
	if err != nil {
		// If the cluster doesn't exist, that's fine
		if _, writeErr := fmt.Fprintf(ginkgo.GinkgoWriter, "No existing cluster to delete\n"); writeErr != nil {
			ginkgo.Fail(fmt.Sprintf("Failed to write to GinkgoWriter: %v", writeErr))
		}
	}

	ginkgo.By("creating a new Kind cluster")
	cmd = exec.Command("kind", "create", "cluster", "--config", "test/e2e/kind-config.yaml")
	_, err = utils.Run(cmd)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred(), "Failed to create Kind cluster")
}

// buildAndLoadOperatorImage builds the operator Docker image and loads it into the Kind cluster.
// This ensures that the Kind cluster has access to the latest version of the operator for testing.
// The build is skipped when the image already exists locally (e.g. prebuilt
// by CI with layer caching), so the image is built once, not twice.
func buildAndLoadOperatorImage() {
	if !localImageExists(projectImage) {
		ginkgo.By("building the manager(Operator) image")
		cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectImage))
		_, err := utils.Run(cmd)
		gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred(), "Failed to build the manager(Operator) image")
	} else {
		ginkgo.By("reusing the existing manager(Operator) image")
	}

	ginkgo.By("loading the manager(Operator) image on Kind")
	err := utils.LoadImageToKindClusterWithName(projectImage)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred(), "Failed to load the manager(Operator) image into Kind")
}

// localImageExists reports whether the image tag exists in the active
// container runtime, mirroring the Makefile CONTAINER_TOOL priority.
func localImageExists(name string) bool {
	tools := []string{}
	if v, ok := os.LookupEnv("CONTAINER_TOOL"); ok && v != "" {
		tools = append(tools, v)
	} else {
		for _, tool := range []string{"docker", "podman", "container"} {
			if _, err := exec.LookPath(tool); err == nil {
				tools = append(tools, tool)
				break
			}
		}
	}
	for _, tool := range tools {
		cmd := exec.Command(tool, "image", "inspect", name)
		if _, err := utils.Run(cmd); err == nil {
			return true
		}
	}
	return false
}

// deployOperator installs the CRDs and deploys the operator to the Kind cluster.
// It waits for the controller-manager deployment to be ready before proceeding.
func deployOperator() {
	// Install CRDs
	ginkgo.By("installing CRDs")
	cmd := exec.Command("make", "install")
	_, err := utils.Run(cmd)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred(), "Failed to install CRDs")

	// Deploy the controller-manager
	ginkgo.By("deploying the controller-manager")
	cmd = exec.Command("make", "deploy-test", fmt.Sprintf("IMG=%s", projectImage))
	_, err = utils.Run(cmd)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred(), "Failed to deploy the controller-manager")

	// Wait for the controller-manager to be ready
	ginkgo.By("waiting for the controller-manager to be ready")
	cmd = exec.Command("kubectl", "wait",
		"--for=condition=Available",
		"deployment",
		"-n", namespace,
		"heartbeats-operator-controller-manager",
		"--timeout=2m")
	_, err = utils.Run(cmd)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred(), "Failed to wait for the controller-manager to be ready")
}
