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

// Package e2e contains end-to-end tests for the Heartbeats operator.
// These tests verify the complete functionality of the operator in a real Kubernetes environment,
// including controller manager operation, metrics endpoint availability, and Heartbeat resource
// reconciliation with various endpoint configurations.
package e2e

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"

	"github.com/siutsin/heartbeats/internal/controller"
	"github.com/siutsin/heartbeats/test/utils"
)

// namespace where the project is deployed in
const namespace = "heartbeats-operator-system"

// serviceAccountName created for the project
const serviceAccountName = "heartbeats-operator-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "heartbeats-operator-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "heartbeats-operator-metrics-binding"

// ManagerTestSuite contains all the e2e tests related to the controller manager functionality.
// This includes tests for basic manager operation, metrics endpoint availability, and controller health.
var _ = ginkgo.Describe("Manager", ginkgo.Ordered, func() {
	var controllerPodName string

	// BeforeAll sets up the test environment before any manager tests run.
	// It configures the namespace with restricted security policy for testing.
	ginkgo.BeforeAll(func() {
		ginkgo.By("labeling the namespace to enforce the restricted security policy")
		cmd := exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err := utils.Run(cmd)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to label namespace with restricted policy")
	})

	// AfterEach collects diagnostic information when tests fail.
	// It fetches logs, events, and pod descriptions to help with debugging.
	ginkgo.AfterEach(func() {
		specReport := ginkgo.CurrentSpecReport()
		if specReport.Failed() {
			collectManagerDiagnosticInfo(controllerPodName)
		}
	})

	ginkgo.Context("Manager", func() {
		// TestManagerBasicOperation verifies that the controller manager pod is running correctly.
		// It checks that the pod exists, is in Running state, and has the expected name format.
		ginkgo.It("should run successfully", func() {
			ginkgo.By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g gomega.Gomega) {
				controllerPodName = getControllerPodName(g)
				verifyPodStatus(g, controllerPodName)
			}
			gomega.Eventually(verifyControllerUp).Should(gomega.Succeed())
		})

		// TestMetricsEndpoint verifies that the metrics endpoint is properly configured and accessible.
		// It sets up RBAC, creates a test pod to access metrics, and validates the metrics output.
		ginkgo.It("should ensure the metrics endpoint is serving metrics", func() {
			setupMetricsAccess()
			verifyMetricsAvailability(controllerPodName)
			createMetricsTestPod()
			verifyMetricsOutput()
		})
	})
})

// collectManagerDiagnosticInfo gathers logs, events, and pod descriptions for debugging failed manager tests.
// This function is called when a test fails to provide diagnostic information.
//
// Parameters:
//   - controllerPodName: The name of the controller pod to collect logs from
func collectManagerDiagnosticInfo(controllerPodName string) {
	ginkgo.By("Fetching controller manager pod logs")
	cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
	controllerLogs, err := utils.Run(cmd)
	if err == nil {
		_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "Controller logs:\n %s", controllerLogs)
	} else {
		_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "Failed to get Controller logs: %s", err)
	}

	ginkgo.By("Fetching Kubernetes events")
	cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
	eventsOutput, err := utils.Run(cmd)
	if err == nil {
		_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
	} else {
		_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "Failed to get Kubernetes events: %s", err)
	}

	ginkgo.By("Fetching curl-metrics logs")
	cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	metricsOutput, err := utils.Run(cmd)
	if err == nil {
		_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
	} else {
		_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
	}

	ginkgo.By("Fetching controller manager pod description")
	cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
	podDescription, err := utils.Run(cmd)
	if err == nil {
		fmt.Println("Pod description:\n", podDescription)
	} else {
		fmt.Println("Failed to describe controller pod")
	}
}

// getControllerPodName retrieves the name of the controller manager pod.
// It filters for pods with the controller-manager label and returns the first active pod.
//
// Parameters:
//   - g: The gomega assertion interface for test assertions
//
// Returns:
//   - string: The name of the controller pod
func getControllerPodName(g gomega.Gomega) string {
	cmd := exec.Command("kubectl", "get",
		"pods", "-l", "control-plane=controller-manager",
		"-o", "go-template={{ range .items }}"+
			"{{ if not .metadata.deletionTimestamp }}"+
			"{{ .metadata.name }}"+
			"{{ \"\\n\" }}{{ end }}{{ end }}",
		"-n", namespace,
	)

	podOutput, err := utils.Run(cmd)
	g.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to retrieve controller-manager pod information")
	podNames := utils.GetNonEmptyLines(podOutput)
	g.Expect(podNames).To(gomega.HaveLen(1), "expected 1 controller pod running")
	controllerPodName := podNames[0]
	g.Expect(controllerPodName).To(gomega.ContainSubstring("controller-manager"))
	return controllerPodName
}

// verifyPodStatus checks that the specified pod is in Running state.
// It validates the pod's phase to ensure it's healthy and operational.
//
// Parameters:
//   - g: The gomega assertion interface for test assertions
//   - podName: The name of the pod to verify
func verifyPodStatus(g gomega.Gomega, podName string) {
	cmd := exec.Command("kubectl", "get",
		"pods", podName, "-o", "jsonpath={.status.phase}",
		"-n", namespace,
	)
	output, err := utils.Run(cmd)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(output).To(gomega.Equal("Running"), "Incorrect controller-manager pod status")
}

// setupMetricsAccess creates the necessary RBAC resources to access the metrics endpoint.
// It creates a ClusterRoleBinding that allows the service account to read metrics.
func setupMetricsAccess() {
	ginkgo.By("creating a ClusterRoleBinding for the service account to allow access to metrics")
	cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
		"--clusterrole=heartbeats-operator-metrics-reader",
		fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
	)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create ClusterRoleBinding")
}

// verifyMetricsAvailability checks that the metrics service exists and is properly configured.
// It validates that the metrics service is available and the endpoint is ready.
//
// Parameters:
//   - controllerPodName: The name of the controller pod to check logs from
func verifyMetricsAvailability(controllerPodName string) {
	ginkgo.By("validating that the metrics service is available")
	cmd := exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Metrics service should exist")

	ginkgo.By("waiting for the metrics endpoint to be ready")
	verifyMetricsEndpointReady := func(g gomega.Gomega) {
		cmd := exec.Command("kubectl", "get", "endpoints", metricsServiceName, "-n", namespace)
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(output).To(gomega.ContainSubstring("8443"), "Metrics endpoint is not ready")
	}
	gomega.Eventually(verifyMetricsEndpointReady).Should(gomega.Succeed())

	ginkgo.By("verifying that the controller manager is serving the metrics server")
	verifyMetricsServerStarted := func(g gomega.Gomega) {
		cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(output).To(gomega.ContainSubstring("controller-runtime.metrics\tServing metrics server"),
			"Metrics server not yet started")
	}
	gomega.Eventually(verifyMetricsServerStarted).Should(gomega.Succeed())
}

// createMetricsTestPod creates a temporary pod to test access to the metrics endpoint.
// It uses a curl image to make requests to the metrics service and verify connectivity.
func createMetricsTestPod() {
	ginkgo.By("getting the service account token")
	token, err := serviceAccountToken()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(token).NotTo(gomega.BeEmpty())

	ginkgo.By("creating the curl-metrics pod to access the metrics endpoint")
	cmd := exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
		"--namespace", namespace,
		"--image=curlimages/curl:latest",
		"--overrides",
		fmt.Sprintf(`{
			"spec": {
				"containers": [{
					"name": "curl",
					"image": "curlimages/curl:latest",
					"command": ["/bin/sh", "-c"],
					"args": ["curl -v -k -H \"Authorization: Bearer $TOKEN\" https://%s.%s.svc.cluster.local:8443/metrics"],
					"env": [{
						"name": "TOKEN",
						"value": "%s"
					}],
					"securityContext": {
						"allowPrivilegeEscalation": false,
						"capabilities": {
							"drop": ["ALL"]
						},
						"runAsNonRoot": true,
						"runAsUser": 1000,
						"seccompProfile": {
							"type": "RuntimeDefault"
						}
					}
				}],
				"serviceAccount": "%s"
			}
		}`, metricsServiceName, namespace, token, serviceAccountName))
	_, err = utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create curl-metrics pod")

	ginkgo.By("waiting for the curl-metrics pod to complete")
	verifyCurlUp := func(g gomega.Gomega) {
		cmd = exec.Command("kubectl", "get", "pods", "curl-metrics",
			"-o", "jsonpath={.status.phase}",
			"-n", namespace)
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(output).To(gomega.Equal("Succeeded"), "curl pod in wrong status")
	}
	gomega.Eventually(verifyCurlUp, 5*time.Minute, 5*time.Second).Should(gomega.Succeed())
}

// verifyMetricsOutput retrieves and validates the metrics output from the test pod.
// It checks that the metrics contain expected controller runtime metrics.
func verifyMetricsOutput() {
	ginkgo.By("getting the metrics by checking curl-metrics logs")
	metricsOutput := getMetricsOutput()
	gomega.Expect(metricsOutput).To(gomega.ContainSubstring(
		"controller_runtime_reconcile_total",
	))
}

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}
	defer func() {
		if err := os.Remove(tokenRequestFile); err != nil {
			ginkgo.Fail(fmt.Sprintf("Failed to remove token request file: %v", err))
		}
	}()

	// Execute kubectl command to create the token
	cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
		"/api/v1/namespaces/%s/serviceaccounts/%s/token",
		namespace,
		serviceAccountName,
	), "-f", tokenRequestFile)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create token: %v", err)
	}

	// Parse the JSON output to extract the token
	var token tokenRequest
	if err := json.Unmarshal(output, &token); err != nil {
		return "", fmt.Errorf("failed to parse token response: %v", err)
	}

	if token.Status.Token == "" {
		return "", fmt.Errorf("received empty token")
	}

	return token.Status.Token, nil
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() string {
	ginkgo.By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	metricsOutput, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to retrieve logs from curl pod")
	gomega.Expect(metricsOutput).To(gomega.ContainSubstring("< HTTP/1.1 200 OK"))
	return metricsOutput
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}

// HeartbeatTestSuite contains all the e2e tests related to Heartbeat resource functionality.
// This includes tests for health checking, status updates, and various endpoint configurations.
var _ = ginkgo.Describe("Heartbeat", ginkgo.Ordered, func() {
	// Test constants for Heartbeat resources
	const (
		heartbeatName            = "test-heartbeat"
		healthySecretName        = "heartbeat-endpoints-healthy"
		unhealthySecretName      = "heartbeat-endpoints-unhealthy"
		invalidSecretName        = "heartbeat-endpoints-invalid"
		missingKeysSecretName    = "heartbeat-endpoints-missing-keys"
		multipleRangesSecretName = "heartbeat-endpoints-multiple-ranges"
		timeoutSecretName        = "heartbeat-endpoints-timeout"
	)

	// BeforeAll sets up the test environment for Heartbeat tests.
	// It creates the initial healthy endpoints secret used by multiple tests.
	ginkgo.BeforeAll(func() {
		createInitialHealthySecret(healthySecretName)
	})

	// AfterEach cleans up Heartbeat resources after each test.
	// It ensures that test resources are properly removed to avoid interference.
	ginkgo.AfterEach(func() {
		cleanupHeartbeatResources(heartbeatName)
	})

	ginkgo.Context("Health Check", func() {
		// TestHealthyEndpoints verifies that Heartbeat resources with healthy endpoints
		// are correctly marked as healthy and have successful report status.
		ginkgo.It("should create and reconcile a healthy Heartbeat", func() {
			createHealthyHeartbeat(heartbeatName, healthySecretName)
			verifyHeartbeatHealth(heartbeatName, true, "Success")
			verifyHeartbeatMessage(heartbeatName, controller.ErrEndpointHealthy)
		})

		// TestUnhealthyEndpoints verifies that Heartbeat resources with unhealthy endpoints
		// are correctly marked as unhealthy and have successful report status.
		ginkgo.It("should handle unhealthy endpoints correctly", func() {
			createUnhealthyEndpointsSecret(unhealthySecretName)
			createUnhealthyHeartbeat(heartbeatName, unhealthySecretName)
			verifyHeartbeatHealth(heartbeatName+"-unhealthy", false, "Failure")
		})

		// TestInvalidEndpointURLs verifies that Heartbeat resources with invalid endpoint URLs
		// are handled gracefully and marked as unhealthy.
		ginkgo.It("should handle invalid endpoint URLs", func() {
			createInvalidEndpointSecret(invalidSecretName)
			verifySecretExists(invalidSecretName)
		})

		// TestMissingKeys verifies that Heartbeat resources with missing required keys
		// are handled gracefully and marked as unhealthy.
		ginkgo.It("should handle missing secret keys", func() {
			createMissingKeysSecret(missingKeysSecretName)
			createMissingKeysHeartbeat(heartbeatName, missingKeysSecretName)
			verifyHeartbeatHealth(heartbeatName, false, "")
			verifyHeartbeatMessageContains(heartbeatName, "missing required key")
		})

		// TestInvalidStatusCodes verifies that Heartbeat resources with invalid status code ranges
		// are handled gracefully and marked as unhealthy.
		ginkgo.It("should handle invalid status code ranges", func() {
			createInvalidStatusCodeHeartbeat(heartbeatName, healthySecretName)
			verifyHeartbeatExists(heartbeatName)
			verifyHeartbeatHealth(heartbeatName, false, "")
			verifyHeartbeatMessage(heartbeatName, controller.ErrInvalidStatusCodeRange)
		})

		// TestMultipleRanges verifies that Heartbeat resources with multiple status code ranges
		// are processed correctly and marked appropriately.
		ginkgo.It("should handle multiple status code ranges", func() {
			createMultipleRangesSecret(multipleRangesSecretName)
			createMultipleRangesHeartbeat(heartbeatName, multipleRangesSecretName)
			verifyHeartbeatExists(heartbeatName)
			verifyHeartbeatHealth(heartbeatName, true, "")
			updateSecretToReturn404(multipleRangesSecretName)
			verifyHeartbeatHealth(heartbeatName, true, "")
		})

		// TestTimeout verifies that Heartbeat resources with timeout configurations
		// are handled correctly and marked appropriately.
		ginkgo.It("should handle endpoint timeout", func() {
			createTimeoutSecret(timeoutSecretName)
			createTimeoutHeartbeat(heartbeatName, timeoutSecretName)
			verifyHeartbeatExists(heartbeatName)
			verifyHeartbeatStatusPopulated(heartbeatName)
		})
	})
})

// createInitialHealthySecret creates the initial healthy endpoints secret used by multiple tests.
// This secret is created once in BeforeAll to avoid duplication across tests.
//
// Parameters:
//   - secretName: The name of the secret to create
func createInitialHealthySecret(secretName string) {
	ginkgo.By("creating a secret with endpoints")
	cmd := exec.Command("kubectl", "create", "secret", "generic", secretName,
		"--from-literal=targetEndpoint=https://httpbin.org/status/200",
		"--from-literal=healthyEndpoint=https://httpbin.org/status/200",
		"--from-literal=unhealthyEndpoint=https://httpbin.org/status/200",
		"-n", namespace)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create secret")
}

// cleanupHeartbeatResources removes Heartbeat resources after each test.
// This ensures that tests don't interfere with each other.
//
// Parameters:
//   - heartbeatName: The base name of the Heartbeat resource to clean up
func cleanupHeartbeatResources(heartbeatName string) {
	ginkgo.By("deleting the Heartbeat resources")
	cmd := exec.Command("kubectl", "delete", "heartbeat", heartbeatName, "-n", namespace)
	_, _ = utils.Run(cmd)
	cmd = exec.Command("kubectl", "delete", "heartbeat", heartbeatName+"-unhealthy", "-n", namespace)
	_, _ = utils.Run(cmd)
}

// createHealthyHeartbeat creates a Heartbeat resource that references the healthy endpoints secret.
// It applies the Heartbeat CRD and waits for it to be created.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to create
//   - secretName: The name of the secret containing endpoint configurations
func createHealthyHeartbeat(heartbeatName, secretName string) {
	ginkgo.By("creating a Heartbeat resource")
	heartbeatYAML := fmt.Sprintf(`apiVersion: monitoring.siutsin.com/v1alpha1
kind: Heartbeat
metadata:
  name: %s
  namespace: %s
spec:
  endpointsSecret:
    name: %s
    targetEndpointKey: targetEndpoint
    healthyEndpointKey: healthyEndpoint
    unhealthyEndpointKey: unhealthyEndpoint
  interval: 30s
  expectedStatusCodeRanges:
    - min: 200
      max: 299`, heartbeatName, namespace, secretName)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(heartbeatYAML)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create Heartbeat")
}

// createUnhealthyEndpointsSecret creates a secret with unhealthy endpoint configurations.
// The endpoints are configured to return HTTP 500, which should mark the heartbeat as unhealthy.
//
// Parameters:
//   - secretName: The name of the secret to create
func createUnhealthyEndpointsSecret(secretName string) {
	ginkgo.By("creating a secret with unhealthy endpoints")
	cmd := exec.Command("kubectl", "create", "secret", "generic", secretName,
		"--from-literal=targetEndpoint=https://httpbin.org/status/500",
		"--from-literal=healthyEndpoint=https://httpbin.org/status/200",
		"--from-literal=unhealthyEndpoint=https://httpbin.org/status/500",
		"-n", namespace)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create secret")
}

// createUnhealthyHeartbeat creates a Heartbeat resource that references the unhealthy endpoints secret.
// It applies the Heartbeat CRD and waits for it to be created.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to create
//   - secretName: The name of the secret containing endpoint configurations
func createUnhealthyHeartbeat(heartbeatName, secretName string) {
	ginkgo.By("creating a Heartbeat resource")
	heartbeatYAML := fmt.Sprintf(`apiVersion: monitoring.siutsin.com/v1alpha1
kind: Heartbeat
metadata:
  name: %s-unhealthy
  namespace: %s
spec:
  endpointsSecret:
    name: %s
    targetEndpointKey: targetEndpoint
    healthyEndpointKey: healthyEndpoint
    unhealthyEndpointKey: unhealthyEndpoint
  interval: 30s
  expectedStatusCodeRanges:
    - min: 200
      max: 299`, heartbeatName, namespace, secretName)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(heartbeatYAML)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create Heartbeat")
}

// createInvalidEndpointSecret creates a secret with an invalid endpoint URL.
// This tests the controller's handling of malformed endpoint specifications.
//
// Parameters:
//   - secretName: The name of the secret to create
func createInvalidEndpointSecret(secretName string) {
	ginkgo.By("creating a secret with an invalid endpoint URL")
	secretYAML := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
data:
  targetEndpoint: %s
  healthyEndpoint: %s
  unhealthyEndpoint: %s`, secretName, namespace,
		base64.StdEncoding.EncodeToString([]byte("")),
		base64.StdEncoding.EncodeToString([]byte("https://httpbin.org/status/200")),
		base64.StdEncoding.EncodeToString([]byte("https://httpbin.org/status/200")))

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(secretYAML)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create secret")
}

// verifySecretExists checks that the specified secret was created successfully.
//
// Parameters:
//   - secretName: The name of the secret to verify
func verifySecretExists(secretName string) {
	ginkgo.By("verifying the secret was created")
	cmd := exec.Command("kubectl", "get", "secret", secretName, "-n", namespace)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Secret not found")
}

// createMissingKeysSecret creates a secret with missing required keys.
// This tests the controller's handling of incomplete endpoint configurations.
//
// Parameters:
//   - secretName: The name of the secret to create
func createMissingKeysSecret(secretName string) {
	ginkgo.By("creating a secret with missing keys")
	var cmd *exec.Cmd
	var err error
	var output string

	// First create the secret
	cmd = exec.Command("kubectl", "create", "secret", "generic", secretName,
		"--from-literal=targetEndpoint=https://httpbin.org/status/200",
		"--from-literal=healthyEndpoint=https://httpbin.org/status/200",
		"--from-literal=unhealthyEndpoint=https://httpbin.org/status/200",
		"-n", namespace)
	_, err = utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create secret")

	// Then get the secret in YAML format
	cmd = exec.Command("kubectl", "get", "secret", secretName,
		"-n", namespace, "-o", "yaml")
	output, err = utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// Parse the YAML and modify it to have empty data
	var secretYAML map[string]interface{}
	err = yaml.Unmarshal([]byte(output), &secretYAML)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	secretYAML["data"] = map[string]interface{}{}

	// Convert back to YAML
	modifiedOutput, err := yaml.Marshal(secretYAML)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// Now replace the secret with empty data
	cmd = exec.Command("kubectl", "replace", "-f", "-")
	cmd.Stdin = strings.NewReader(string(modifiedOutput))
	_, err = utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to update secret")
}

// createMissingKeysHeartbeat creates a Heartbeat resource that references the missing keys secret.
// It applies the Heartbeat CRD and waits for it to be created.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to create
//   - secretName: The name of the secret containing endpoint configurations
func createMissingKeysHeartbeat(heartbeatName, secretName string) {
	ginkgo.By("creating a Heartbeat resource")
	heartbeatYAML := fmt.Sprintf(`apiVersion: monitoring.siutsin.com/v1alpha1
kind: Heartbeat
metadata:
  name: %s
  namespace: %s
spec:
  endpointsSecret:
    name: %s
    targetEndpointKey: targetEndpoint
    healthyEndpointKey: healthyEndpoint
    unhealthyEndpointKey: unhealthyEndpoint
  interval: 5s
  expectedStatusCodeRanges:
    - min: 200
      max: 299`, heartbeatName, namespace, secretName)
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(heartbeatYAML)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create Heartbeat resource")
}

// createInvalidStatusCodeHeartbeat creates a Heartbeat resource with invalid status code ranges.
// This tests the controller's handling of malformed status code specifications.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to create
//   - secretName: The name of the secret containing endpoint configurations
func createInvalidStatusCodeHeartbeat(heartbeatName, secretName string) {
	ginkgo.By("creating a Heartbeat with invalid status code ranges")
	heartbeatYAML := fmt.Sprintf(`apiVersion: monitoring.siutsin.com/v1alpha1
kind: Heartbeat
metadata:
  name: %s
  namespace: %s
spec:
  endpointsSecret:
    name: %s
    targetEndpointKey: targetEndpoint
    healthyEndpointKey: healthyEndpoint
    unhealthyEndpointKey: unhealthyEndpoint
  interval: 5s
  expectedStatusCodeRanges:
    - min: 300
      max: 200`, heartbeatName, namespace, secretName)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(heartbeatYAML)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create Heartbeat")
}

// createMultipleRangesSecret creates a secret with multiple status code range configurations.
// This tests the controller's handling of complex status code specifications.
//
// Parameters:
//   - secretName: The name of the secret to create
func createMultipleRangesSecret(secretName string) {
	ginkgo.By("creating a secret with multiple status code ranges")
	cmd := exec.Command("kubectl", "create", "secret", "generic", secretName,
		"--from-literal=targetEndpoint=https://httpbin.org/status/200",
		"--from-literal=healthyEndpoint=https://httpbin.org/status/200",
		"--from-literal=unhealthyEndpoint=https://httpbin.org/status/200",
		"-n", namespace)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create secret")
}

// createMultipleRangesHeartbeat creates a Heartbeat resource that references the multiple ranges secret.
// It applies the Heartbeat CRD and waits for it to be created.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to create
//   - secretName: The name of the secret containing endpoint configurations
func createMultipleRangesHeartbeat(heartbeatName, secretName string) {
	ginkgo.By("creating a Heartbeat resource")
	heartbeatYAML := fmt.Sprintf(`apiVersion: monitoring.siutsin.com/v1alpha1
kind: Heartbeat
metadata:
  name: %s
  namespace: %s
spec:
  endpointsSecret:
    name: %s
    targetEndpointKey: targetEndpoint
    healthyEndpointKey: healthyEndpoint
    unhealthyEndpointKey: unhealthyEndpoint
  interval: 5s
  expectedStatusCodeRanges:
    - min: 200
      max: 299
    - min: 404
      max: 404`, heartbeatName, namespace, secretName)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(heartbeatYAML)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create Heartbeat")
}

// createTimeoutSecret creates a secret with timeout configurations.
// This tests the controller's handling of timeout settings.
//
// Parameters:
//   - secretName: The name of the secret to create
func createTimeoutSecret(secretName string) {
	ginkgo.By("creating a secret with a timeout endpoint")
	cmd := exec.Command("kubectl", "create", "secret", "generic", secretName,
		"--from-literal=targetEndpoint=https://httpbin.org/delay/15",
		"--from-literal=healthyEndpoint=https://httpbin.org/status/200",
		"--from-literal=unhealthyEndpoint=https://httpbin.org/status/200",
		"-n", namespace)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create secret")
}

// createTimeoutHeartbeat creates a Heartbeat resource that references the timeout secret.
// It applies the Heartbeat CRD and waits for it to be created.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to create
//   - secretName: The name of the secret containing endpoint configurations
func createTimeoutHeartbeat(heartbeatName, secretName string) {
	ginkgo.By("creating a Heartbeat resource")
	heartbeatYAML := fmt.Sprintf(`apiVersion: monitoring.siutsin.com/v1alpha1
kind: Heartbeat
metadata:
  name: %s
  namespace: %s
spec:
  endpointsSecret:
    name: %s
    targetEndpointKey: targetEndpoint
    healthyEndpointKey: healthyEndpoint
    unhealthyEndpointKey: unhealthyEndpoint
  interval: 5s
  expectedStatusCodeRanges:
    - min: 200
      max: 299`, heartbeatName, namespace, secretName)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(heartbeatYAML)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create Heartbeat")
}

// verifyHeartbeatExists checks that the specified Heartbeat resource exists.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to verify
func verifyHeartbeatExists(heartbeatName string) {
	ginkgo.By("waiting for the Heartbeat resource to be ready")
	verifyHeartbeatExists := func(g gomega.Gomega) {
		cmd := exec.Command("kubectl", "get", "heartbeat", heartbeatName, "-n", namespace)
		_, err := utils.Run(cmd)
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}
	gomega.Eventually(verifyHeartbeatExists, 10*time.Second, 3*time.Second).Should(gomega.Succeed())
}

// verifyHeartbeatHealth verifies that the Heartbeat resource has the expected health status and report status.
// It polls the Heartbeat status until the expected values are reached or timeout occurs.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to verify
//   - expectedHealthy: The expected health status (true for healthy, false for unhealthy)
//   - expectedReportStatus: The expected report status (e.g., "Success", "Failure")
func verifyHeartbeatHealth(heartbeatName string, expectedHealthy bool, expectedReportStatus string) {
	ginkgo.By("verifying the Heartbeat status")
	verifyHeartbeatStatus := func(g gomega.Gomega) {
		cmd := exec.Command("kubectl", "get", "heartbeat", heartbeatName,
			"-o", "jsonpath={.status.healthy}",
			"-n", namespace)
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		healthy := output == "true"
		g.Expect(healthy).To(gomega.Equal(expectedHealthy), "Heartbeat health status mismatch")

		if expectedReportStatus != "" {
			// Check reportStatus
			cmd = exec.Command("kubectl", "get", "heartbeat", heartbeatName,
				"-o", "jsonpath={.status.reportStatus}",
				"-n", namespace)
			reportStatus, err := utils.Run(cmd)
			g.Expect(err).NotTo(gomega.HaveOccurred())
			g.Expect(reportStatus).To(gomega.Equal(expectedReportStatus), "reportStatus mismatch")
		}
	}
	gomega.Eventually(verifyHeartbeatStatus, 30*time.Second, 5*time.Second).Should(gomega.Succeed())
}

// verifyHeartbeatMessage verifies that the Heartbeat resource has the expected status message.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to verify
//   - expectedMessage: The expected status message
func verifyHeartbeatMessage(heartbeatName, expectedMessage string) {
	ginkgo.By("verifying the Heartbeat message contains the correct status code")
	cmd := exec.Command("kubectl", "get", "heartbeat", heartbeatName,
		"-o", "jsonpath={.status.message}",
		"-n", namespace)
	output, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(output).To(gomega.Equal(expectedMessage))
}

// verifyHeartbeatMessageContains verifies that the Heartbeat resource's status message contains the expected substring.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to verify
//   - expectedSubstring: The expected substring in the status message
func verifyHeartbeatMessageContains(heartbeatName, expectedSubstring string) {
	ginkgo.By("verifying the Heartbeat message indicates missing keys")
	cmd := exec.Command("kubectl", "get", "heartbeat", heartbeatName,
		"-o", "jsonpath={.status.message}",
		"-n", namespace)
	output, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(output).To(gomega.ContainSubstring(expectedSubstring))
}

// updateSecretToReturn404 updates the secret to return a 404 status code.
// This tests the controller's handling of different status codes within the valid range.
//
// Parameters:
//   - secretName: The name of the secret to update
func updateSecretToReturn404(secretName string) {
	ginkgo.By("updating the secret to return 404 status code")
	cmd := exec.Command("kubectl", "patch", "secret", secretName,
		"--type=merge", "-p", `{"data": {
			"targetEndpoint": "aHR0cHM6Ly9odHRwYmluLm9yZy9zdGF0dXMvNDA0",
			"healthyEndpoint": "aHR0cHM6Ly9odHRwYmluLm9yZy9zdGF0dXMvMjAw",
			"unhealthyEndpoint": "aHR0cHM6Ly9odHRwYmluLm9yZy9zdGF0dXMvMjAw"
		}}`, "-n", namespace)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to update secret")
}

// verifyHeartbeatStatusPopulated verifies that the Heartbeat resource's status message is populated.
// This is used for tests where we expect the status to be set but don't care about the specific value.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to verify
func verifyHeartbeatStatusPopulated(heartbeatName string) {
	ginkgo.By("waiting for the Heartbeat status to be populated")
	verifyHeartbeatStatusPopulated := func(g gomega.Gomega) {
		cmd := exec.Command("kubectl", "get", "heartbeat", heartbeatName,
			"-o", "jsonpath={.status.message}",
			"-n", namespace)
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(output).NotTo(gomega.BeEmpty(), "Heartbeat status message should be populated")
	}
	gomega.Eventually(verifyHeartbeatStatusPopulated, 60*time.Second, 5*time.Second).Should(gomega.Succeed())
}
