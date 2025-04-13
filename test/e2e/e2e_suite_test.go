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

package e2e

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/siutsin/heartbeats/test/utils"
)

var (
	projectImage = "heartbeats-operator:test"
)

func TestE2E(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "E2E Suite")
}

var _ = ginkgo.BeforeSuite(func() {
	ginkgo.By("Setting up test environment")
	ginkgo.By("deleting any existing Kind cluster")
	cmd := exec.Command("kind", "delete", "cluster")
	_, err := utils.Run(cmd)
	if err != nil {
		// If the cluster doesn't exist, that's fine
		if _, err := fmt.Fprintf(ginkgo.GinkgoWriter, "No existing cluster to delete\n"); err != nil {
			ginkgo.Fail(fmt.Sprintf("Failed to write to GinkgoWriter: %v", err))
		}
	}

	ginkgo.By("creating a new Kind cluster")
	cmd = exec.Command("kind", "create", "cluster", "--config", "test/e2e/kind-config.yaml")
	_, err = utils.Run(cmd)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred(), "Failed to create Kind cluster")

	ginkgo.By("building the manager(Operator) image")
	cmd = exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectImage))
	_, err = utils.Run(cmd)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred(), "Failed to build the manager(Operator) image")

	ginkgo.By("loading the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectImage)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred(), "Failed to load the manager(Operator) image into Kind")

	// Install CRDs
	ginkgo.By("installing CRDs")
	cmd = exec.Command("make", "install")
	_, err = utils.Run(cmd)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred(), "Failed to install CRDs")

	// Deploy the controller-manager
	ginkgo.By("deploying the controller-manager")
	cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
	_, err = utils.Run(cmd)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred(), "Failed to deploy the controller-manager")

	// Wait for the controller-manager to be ready
	ginkgo.By("waiting for the controller-manager to be ready")
	cmd = exec.Command("kubectl", "wait",
		"--for=condition=Available",
		"deployment",
		"-n", namespace,
		"heartbeats-operator-controller-manager",
		"--timeout=5m")
	_, err = utils.Run(cmd)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred(), "Failed to wait for the controller-manager to be ready")

	// Install CertManager if not already installed
	ginkgo.By("checking if CertManager is already installed")
	if !utils.IsCertManagerCRDsInstalled() {
		msg := "Installing CertManager...\n"
		if _, err := fmt.Fprint(ginkgo.GinkgoWriter, msg); err != nil {
			ginkgo.Fail(fmt.Sprintf("Failed to write to GinkgoWriter: %v", err))
		}
		gomega.Expect(utils.InstallCertManager()).To(gomega.Succeed(), "Failed to install CertManager")
	} else {
		msg := "WARNING: CertManager is already installed. Skipping installation...\n"
		if _, err := fmt.Fprint(ginkgo.GinkgoWriter, msg); err != nil {
			ginkgo.Fail(fmt.Sprintf("Failed to write to GinkgoWriter: %v", err))
		}
	}
})

var _ = ginkgo.AfterSuite(func() {
	ginkgo.By("Tearing down test environment")
	ginkgo.By("deleting Kind cluster")
	cmd := exec.Command("kind", "delete", "cluster")
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to delete Kind cluster")
})
