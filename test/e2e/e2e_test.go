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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

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

var _ = ginkgo.Describe("Manager", ginkgo.Ordered, func() {
	var controllerPodName string

	// Before running the tests, create a kind cluster and set up the environment
	ginkgo.BeforeAll(func() {
		ginkgo.By("labeling the namespace to enforce the restricted security policy")
		cmd := exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err := utils.Run(cmd)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to label namespace with restricted policy")
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	ginkgo.AfterEach(func() {
		specReport := ginkgo.CurrentSpecReport()
		if specReport.Failed() {
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
	})

	ginkgo.Context("Manager", func() {
		ginkgo.It("should run successfully", func() {
			ginkgo.By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g gomega.Gomega) {
				// Get the name of the controller-manager pod
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
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(gomega.ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(output).To(gomega.Equal("Running"), "Incorrect controller-manager pod status")
			}
			gomega.Eventually(verifyControllerUp).Should(gomega.Succeed())
		})

		ginkgo.It("should ensure the metrics endpoint is serving metrics", func() {
			ginkgo.By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=heartbeats-operator-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create ClusterRoleBinding")

			ginkgo.By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Metrics service should exist")

			ginkgo.By("getting the service account token")
			token, err := serviceAccountToken()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(token).NotTo(gomega.BeEmpty())

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

			ginkgo.By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
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
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(output).To(gomega.Equal("Succeeded"), "curl pod in wrong status")
			}
			gomega.Eventually(verifyCurlUp, 5*time.Minute).Should(gomega.Succeed())

			ginkgo.By("getting the metrics by checking curl-metrics logs")
			metricsOutput := getMetricsOutput()
			gomega.Expect(metricsOutput).To(gomega.ContainSubstring(
				"controller_runtime_reconcile_total",
			))
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		// TODO: Customize the e2e test suite with scenarios specific to your project.
		// Consider applying sample/CR(s) and check their status and/or verifying
		// the reconciliation by using the metrics, i.e.:
		// metricsOutput := getMetricsOutput()
		// Expect(metricsOutput).To(ContainSubstring(
		//    fmt.Sprintf(`controller_runtime_reconcile_total{controller="%s",result="success"} 1`,
		//    strings.ToLower(<Kind>),
		// ))
	})

	ginkgo.Context("Heartbeat", ginkgo.Ordered, func() {
		const (
			heartbeatName            = "test-heartbeat"
			healthySecretName        = "heartbeat-endpoints-healthy"
			unhealthySecretName      = "heartbeat-endpoints-unhealthy"
			invalidSecretName        = "heartbeat-endpoints-invalid"
			missingKeysSecretName    = "heartbeat-endpoints-missing-keys"
			multipleRangesSecretName = "heartbeat-endpoints-multiple-ranges"
			timeoutSecretName        = "heartbeat-endpoints-timeout"
		)

		ginkgo.BeforeAll(func() {
			ginkgo.By("creating a secret with endpoints")
			cmd := exec.Command("kubectl", "create", "secret", "generic", healthySecretName,
				"--from-literal=targetEndpoint=https://httpbin.org/status/200",
				"--from-literal=healthyEndpoint=https://httpbin.org/status/200",
				"--from-literal=unhealthyEndpoint=https://httpbin.org/status/200",
				"-n", namespace)
			_, err := utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create secret")
		})

		ginkgo.AfterEach(func() {
			ginkgo.By("deleting the Heartbeat resource")
			cmd := exec.Command("kubectl", "delete", "heartbeat", heartbeatName, "-n", namespace)
			_, _ = utils.Run(cmd)
		})

		ginkgo.It("should create and reconcile a healthy Heartbeat", func() {
			var cmd *exec.Cmd
			var err error
			var output string

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
      max: 299`, heartbeatName, namespace, healthySecretName)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(heartbeatYAML)
			_, err = utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create Heartbeat")

			ginkgo.By("verifying the Heartbeat status becomes Healthy")
			verifyHeartbeatHealthy := func(g gomega.Gomega) {
				cmd := exec.Command("kubectl", "get", "heartbeat", heartbeatName,
					"-o", "jsonpath={.status.healthy}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(output).To(gomega.Equal("true"), "Heartbeat status is not Healthy")
			}
			gomega.Eventually(verifyHeartbeatHealthy, 30*time.Second, 5*time.Second).Should(gomega.Succeed())

			ginkgo.By("verifying the Heartbeat message contains the correct status code")
			cmd = exec.Command("kubectl", "get", "heartbeat", heartbeatName,
				"-o", "jsonpath={.status.message}",
				"-n", namespace)
			output, err = utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.Equal(controller.ErrEndpointHealthy))
		})

		ginkgo.It("should handle unhealthy endpoints correctly", func() {
			var output string
			var err error
			var cmd *exec.Cmd
			ginkgo.By("creating a secret with unhealthy endpoints")
			cmd = exec.Command("kubectl", "create", "secret", "generic", unhealthySecretName,
				"--from-literal=targetEndpoint=https://httpbin.org/status/500",
				"--from-literal=healthyEndpoint=https://httpbin.org/status/200",
				"--from-literal=unhealthyEndpoint=https://httpbin.org/status/200",
				"-n", namespace)
			output, err = utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create secret")

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
      max: 299`, heartbeatName, namespace, unhealthySecretName)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(heartbeatYAML)
			_, err = utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create Heartbeat")

			ginkgo.By("verifying the Heartbeat status becomes Unhealthy")
			verifyHeartbeatUnhealthy := func(g gomega.Gomega) {
				cmd = exec.Command("kubectl", "get", "heartbeat", heartbeatName,
					"-o", "jsonpath={.status.healthy}",
					"-n", namespace)
				output, err = utils.Run(cmd)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(output).To(gomega.Equal("false"), "Heartbeat status is not Unhealthy")

				cmd = exec.Command("kubectl", "get", "heartbeat", heartbeatName,
					"-o", "jsonpath={.status.message}",
					"-n", namespace)
				output, err = utils.Run(cmd)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(output).To(gomega.Equal(controller.ErrStatusCodeNotInRange))
			}
			gomega.Eventually(verifyHeartbeatUnhealthy, 10*time.Second, time.Second).Should(gomega.Succeed())
		})

		ginkgo.It("should handle invalid endpoint URLs", func() {
			var err error
			var cmd *exec.Cmd

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
  unhealthyEndpoint: %s`, invalidSecretName, namespace,
				base64.StdEncoding.EncodeToString([]byte("")),
				base64.StdEncoding.EncodeToString([]byte("https://httpbin.org/status/200")),
				base64.StdEncoding.EncodeToString([]byte("https://httpbin.org/status/200")))

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(secretYAML)
			_, err = utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create secret")

			ginkgo.By("verifying the secret was created")
			cmd = exec.Command("kubectl", "get", "secret", invalidSecretName, "-n", namespace)
			_, err = utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Secret not found")
		})

		ginkgo.It("should handle missing secret keys", func() {
			ginkgo.By("creating a secret with missing keys")
			var cmd *exec.Cmd
			var err error
			var output string

			// First create the secret
			cmd = exec.Command("kubectl", "create", "secret", "generic", missingKeysSecretName,
				"--from-literal=targetEndpoint=https://httpbin.org/status/200",
				"--from-literal=healthyEndpoint=https://httpbin.org/status/200",
				"--from-literal=unhealthyEndpoint=https://httpbin.org/status/200",
				"-n", namespace)
			_, err = utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create secret")

			// Then get the secret in YAML format
			cmd = exec.Command("kubectl", "get", "secret", missingKeysSecretName,
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
      max: 299`, heartbeatName, namespace, missingKeysSecretName)
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(heartbeatYAML)
			_, err = utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create Heartbeat resource")

			ginkgo.By("verifying the Heartbeat status becomes Unhealthy due to missing keys")
			verifyHeartbeatUnhealthy := func(g gomega.Gomega) {
				cmd = exec.Command("kubectl", "get", "heartbeat", heartbeatName,
					"-o", "jsonpath={.status.healthy}",
					"-n", namespace)
				output, err = utils.Run(cmd)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(output).To(gomega.Equal("false"), "Heartbeat status is not Unhealthy")
			}
			gomega.Eventually(verifyHeartbeatUnhealthy, 10*time.Second, 1*time.Second).Should(gomega.Succeed())

			ginkgo.By("verifying the Heartbeat message indicates missing keys")
			cmd = exec.Command("kubectl", "get", "heartbeat", heartbeatName,
				"-o", "jsonpath={.status.message}",
				"-n", namespace)
			output, err = utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.Equal(controller.ErrMissingRequiredKey))
		})

		ginkgo.It("should handle invalid status code ranges", func() {
			var output string
			var err error
			var cmd *exec.Cmd
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
  interval: 30s
  expectedStatusCodeRanges:
    - min: 300
      max: 200`, heartbeatName, namespace, healthySecretName)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(heartbeatYAML)
			_, err = utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create Heartbeat")

			ginkgo.By("waiting for the Heartbeat resource to be ready")
			verifyHeartbeatExists := func(g gomega.Gomega) {
				cmd = exec.Command("kubectl", "get", "heartbeat", heartbeatName,
					"-n", namespace)
				_, err = utils.Run(cmd)
				g.Expect(err).NotTo(gomega.HaveOccurred(), "Heartbeat resource not found")
			}
			gomega.Eventually(verifyHeartbeatExists, 10*time.Second, 1*time.Second).Should(gomega.Succeed())

			ginkgo.By("verifying the Heartbeat status becomes Unhealthy due to invalid ranges")
			verifyHeartbeatUnhealthy := func(g gomega.Gomega) {
				cmd = exec.Command("kubectl", "get", "heartbeat", heartbeatName,
					"-o", "jsonpath={.status.healthy}",
					"-n", namespace)
				output, err = utils.Run(cmd)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(output).To(gomega.Equal("false"), "Heartbeat status is not Unhealthy")
			}
			gomega.Eventually(verifyHeartbeatUnhealthy, 10*time.Second, 1*time.Second).Should(gomega.Succeed())

			ginkgo.By("verifying the Heartbeat message indicates invalid ranges")
			cmd = exec.Command("kubectl", "get", "heartbeat", heartbeatName,
				"-o", "jsonpath={.status.message}",
				"-n", namespace)
			output, err = utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.Equal(controller.ErrInvalidStatusCodeRange))
		})

		ginkgo.It("should handle multiple status code ranges", func() {
			ginkgo.By("creating a secret with multiple status code ranges")
			var cmd *exec.Cmd
			var err error
			var output string
			cmd = exec.Command("kubectl", "create", "secret", "generic", multipleRangesSecretName,
				"--from-literal=targetEndpoint=https://httpbin.org/status/200",
				"--from-literal=healthyEndpoint=https://httpbin.org/status/200",
				"--from-literal=unhealthyEndpoint=https://httpbin.org/status/200",
				"-n", namespace)
			_, err = utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

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
      max: 299
    - min: 400
      max: 499`, heartbeatName, namespace, multipleRangesSecretName)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(heartbeatYAML)
			_, err = utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create Heartbeat")

			ginkgo.By("waiting for the Heartbeat resource to be ready")
			verifyHeartbeatExists := func(g gomega.Gomega) {
				cmd = exec.Command("kubectl", "get", "heartbeat", heartbeatName,
					"-n", namespace)
				_, err = utils.Run(cmd)
				g.Expect(err).NotTo(gomega.HaveOccurred(), "Heartbeat resource not found")
			}
			gomega.Eventually(verifyHeartbeatExists, 10*time.Second, 1*time.Second).Should(gomega.Succeed())

			ginkgo.By("verifying the Heartbeat status becomes Healthy for 200 status code")
			verifyHeartbeatHealthy := func(g gomega.Gomega) {
				cmd = exec.Command("kubectl", "get", "heartbeat", heartbeatName,
					"-o", "jsonpath={.status.healthy}",
					"-n", namespace)
				output, err = utils.Run(cmd)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(output).To(gomega.Equal("true"), "Heartbeat status is not Healthy")
			}
			gomega.Eventually(verifyHeartbeatHealthy, 2*time.Minute, 5*time.Second).Should(gomega.Succeed())

			ginkgo.By("updating the secret to return 404 status code")
			secretPatchYAML := fmt.Sprintf(`{"data": {
				"targetEndpoint": %q,
				"healthyEndpoint": %q,
				"unhealthyEndpoint": %q
			}}`, base64.StdEncoding.EncodeToString([]byte("https://httpbin.org/status/404")),
				base64.StdEncoding.EncodeToString([]byte("https://httpbin.org/status/200")),
				base64.StdEncoding.EncodeToString([]byte("https://httpbin.org/status/200")))
			cmd = exec.Command("kubectl", "patch", "secret", multipleRangesSecretName,
				"--type=merge", "-p", secretPatchYAML,
				"-n", namespace)
			_, err = utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to update secret")

			ginkgo.By("verifying the Heartbeat status becomes Healthy for 404 status code")
			gomega.Eventually(verifyHeartbeatHealthy, 2*time.Minute, 5*time.Second).Should(gomega.Succeed())
		})

		ginkgo.It("should handle endpoint timeout", func() {
			ginkgo.By("creating a secret with a timeout endpoint")
			var cmd *exec.Cmd
			var err error
			var output string
			cmd = exec.Command("kubectl", "create", "secret", "generic", timeoutSecretName,
				"--from-literal=targetEndpoint=https://httpbin.org/delay/15",
				"--from-literal=healthyEndpoint=https://httpbin.org/status/200",
				"--from-literal=unhealthyEndpoint=https://httpbin.org/status/200",
				"-n", namespace)
			_, err = utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

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
      max: 299`, heartbeatName, namespace, timeoutSecretName)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(heartbeatYAML)
			_, err = utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create Heartbeat")

			ginkgo.By("waiting for the Heartbeat resource to be ready")
			verifyHeartbeatExists := func(g gomega.Gomega) {
				cmd = exec.Command("kubectl", "get", "heartbeat", heartbeatName,
					"-n", namespace)
				output, err = utils.Run(cmd)
				g.Expect(err).NotTo(gomega.HaveOccurred(), "Heartbeat resource not found")
			}
			gomega.Eventually(verifyHeartbeatExists, 30*time.Second, 1*time.Second).Should(gomega.Succeed())

			ginkgo.By("waiting for the Heartbeat status to be populated")
			verifyHeartbeatStatusPopulated := func(g gomega.Gomega) {
				cmd = exec.Command("kubectl", "get", "heartbeat", heartbeatName,
					"-o", "jsonpath={.status.healthy}",
					"-n", namespace)
				output, err = utils.Run(cmd)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(output).NotTo(gomega.BeEmpty(), "Heartbeat status has not been populated yet")
			}
			gomega.Eventually(verifyHeartbeatStatusPopulated, 60*time.Second, 1*time.Second).Should(gomega.Succeed())

			ginkgo.By("verifying the Heartbeat message indicates timeout")
			cmd = exec.Command("kubectl", "get", "heartbeat", heartbeatName,
				"-o", "jsonpath={.status.message}",
				"-n", namespace)
			output, err = utils.Run(cmd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.Equal(controller.ErrFailedToCheckEndpoint))
		})
	})
})

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
