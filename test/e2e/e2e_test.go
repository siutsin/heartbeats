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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	heartbeats "github.com/siutsin/heartbeats/internal"
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

const (
	heartbeatTestServerName = "heartbeat-e2e-http"
	heartbeatTestServerPort = 8080
)

// Pinned helper images so kubelet uses IfNotPresent instead of repulling
// :latest on every run.
const (
	// metricsProbeImage is the curl image used to probe the metrics endpoint.
	metricsProbeImage = "curlimages/curl:8.11.1"
	// heartbeatTestServerImage serves /status/:code and /delay/:seconds.
	heartbeatTestServerImage = "python:3.13.7-alpine"
)

// TestManager covers controller manager operation: pod running and metrics endpoint.
func TestManager(t *testing.T) {
	var controllerPodName string

	t.Log("labelling the namespace to enforce the restricted security policy")
	cmd := exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
		"pod-security.kubernetes.io/enforce=restricted")
	_, err := utils.Run(cmd)
	require.NoError(t, err, "Failed to label namespace with restricted policy")

	// verifyControllerUp polls until the controller-manager pod exists and runs.
	t.Run("controller running", func(t *testing.T) {
		t.Log("validating that the controller-manager pod is running as expected")
		require.Eventually(t, func() bool {
			name, err := tryControllerPodName()
			if err != nil {
				return false
			}
			controllerPodName = name
			return podRunning(name)
		}, 60*time.Second, time.Second)
	})

	// verifyMetricsEndpoint polls the metrics service, then a curl pod checks output.
	t.Run("metrics endpoint", func(t *testing.T) {
		setupMetricsAccess(t)
		verifyMetricsAvailability(t, controllerPodName)
		createMetricsTestPod(t)
		verifyMetricsOutput(t)
	})

	if t.Failed() {
		collectManagerDiagnosticInfo(t, controllerPodName)
	}
}

// writeDiagnosticf writes diagnostic output to stdout without changing test control flow.
// Diagnostic logging should never mask the original test failure it is trying to explain.
func writeDiagnosticf(format string, args ...any) {
	fmt.Printf(format, args...)
}

// collectManagerDiagnosticInfo gathers logs, events, and pod descriptions for debugging failed manager tests.
// This function is called when a test fails to provide diagnostic information.
//
// Parameters:
//   - controllerPodName: The name of the controller pod to collect logs from
func collectManagerDiagnosticInfo(t *testing.T, controllerPodName string) {
	t.Log("Fetching controller manager pod logs")
	cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
	controllerLogs, err := utils.Run(cmd)
	if err == nil {
		writeDiagnosticf("Controller logs:\n %s", controllerLogs)
	} else {
		writeDiagnosticf("Failed to get Controller logs: %s", err)
	}

	t.Log("Fetching Kubernetes events")
	cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
	eventsOutput, err := utils.Run(cmd)
	if err == nil {
		writeDiagnosticf("Kubernetes events:\n%s", eventsOutput)
	} else {
		writeDiagnosticf("Failed to get Kubernetes events: %s", err)
	}

	t.Log("Fetching curl-metrics logs")
	cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	metricsOutput, err := utils.Run(cmd)
	if err == nil {
		writeDiagnosticf("Metrics logs:\n %s", metricsOutput)
	} else {
		writeDiagnosticf("Failed to get curl-metrics logs: %s", err)
	}

	t.Log("Fetching controller manager pod description")
	cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
	podDescription, err := utils.Run(cmd)
	if err == nil {
		writeDiagnosticf("Pod description:\n %s", podDescription)
	} else {
		writeDiagnosticf("Failed to describe controller pod")
	}
}

// tryControllerPodName returns the controller-manager pod name when exactly one
// active controller-manager pod exists.
func tryControllerPodName() (string, error) {
	cmd := exec.Command("kubectl", "get",
		"pods", "-l", "control-plane=controller-manager",
		"-o", "go-template={{ range .items }}"+
			"{{ if not .metadata.deletionTimestamp }}"+
			"{{ .metadata.name }}"+
			"{{ \"\\n\" }}{{ end }}{{ end }}",
		"-n", namespace,
	)

	podOutput, err := utils.Run(cmd)
	if err != nil {
		return "", err
	}
	podNames := utils.GetNonEmptyLines(podOutput)
	if len(podNames) != 1 {
		return "", fmt.Errorf("expected 1 controller pod running, got %d", len(podNames))
	}
	if !strings.Contains(podNames[0], "controller-manager") {
		return "", fmt.Errorf("unexpected controller pod name %q", podNames[0])
	}
	return podNames[0], nil
}

// podRunning reports whether the pod is in Running state.
func podRunning(podName string) bool {
	cmd := exec.Command("kubectl", "get",
		"pods", podName, "-o", "jsonpath={.status.phase}",
		"-n", namespace,
	)
	output, err := utils.Run(cmd)
	return err == nil && output == "Running"
}

// setupMetricsAccess creates the necessary RBAC resources to access the metrics endpoint.
// It creates a ClusterRoleBinding that allows the service account to read metrics.
func setupMetricsAccess(t *testing.T) {
	t.Log("creating a ClusterRoleBinding for the service account to allow access to metrics")
	cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
		"--clusterrole=heartbeats-operator-metrics-reader",
		fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
	)
	_, err := utils.Run(cmd)
	require.NoError(t, err, "Failed to create ClusterRoleBinding")
}

// verifyMetricsAvailability checks that the metrics service exists and is properly configured.
// It validates that the metrics service is available and the endpoint is ready.
//
// Parameters:
//   - controllerPodName: The name of the controller pod to check logs from
func verifyMetricsAvailability(t *testing.T, controllerPodName string) {
	t.Log("validating that the metrics service is available")
	cmd := exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
	_, err := utils.Run(cmd)
	require.NoError(t, err, "Metrics service should exist")

	t.Log("waiting for the metrics endpoint to be ready")
	require.Eventually(t, func() bool {
		cmd := exec.Command("kubectl", "get", "endpoints", metricsServiceName, "-n", namespace)
		output, err := utils.Run(cmd)
		return err == nil && strings.Contains(output, "8443")
	}, 60*time.Second, time.Second, "Metrics endpoint is not ready")

	t.Log("verifying that the controller manager is serving the metrics server")
	require.Eventually(t, func() bool {
		cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
		output, err := utils.Run(cmd)
		return err == nil && strings.Contains(output, "Serving metrics server")
	}, 60*time.Second, time.Second, "Metrics server not yet started")
}

// createMetricsTestPod creates a temporary pod to test access to the metrics endpoint.
// It uses a curl image to make requests to the metrics service and verify connectivity.
func createMetricsTestPod(t *testing.T) {
	t.Log("getting the service account token")
	token, err := serviceAccountToken()
	require.NoError(t, err)
	require.NotEmpty(t, token)

	t.Log("creating the curl-metrics pod to access the metrics endpoint")
	cmd := exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
		"--namespace", namespace,
		"--image="+metricsProbeImage,
		"--image-pull-policy=IfNotPresent",
		"--overrides",
		fmt.Sprintf(`{
			"spec": {
				"containers": [{
					"name": "curl",
					"image": "`+metricsProbeImage+`",
					"command": ["/bin/sh", "-c"],
					"args": ["curl -v -k -H \"Authorization: Bearer $TOKEN\" https://%s:8443/metrics"],
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
		}`, metricsServiceName, token, serviceAccountName))
	_, err = utils.Run(cmd)
	require.NoError(t, err, "Failed to create curl-metrics pod")

	t.Log("waiting for the curl-metrics pod to complete")
	require.Eventually(t, func() bool {
		cmd = exec.Command("kubectl", "get", "pods", "curl-metrics",
			"-o", "jsonpath={.status.phase}",
			"-n", namespace)
		output, err := utils.Run(cmd)
		return err == nil && output == "Succeeded"
	}, 2*time.Minute, 2*time.Second, "curl pod in wrong status")
}

// verifyMetricsOutput retrieves and validates the metrics output from the test pod.
// It checks that the metrics contain expected controller runtime metrics.
func verifyMetricsOutput(t *testing.T) {
	t.Log("getting the metrics by checking curl-metrics logs")
	metricsOutput := getMetricsOutput(t)
	require.Contains(t, metricsOutput, "controller_runtime_reconcile_total")
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

	// Execute kubectl command to create the token
	cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
		"/api/v1/namespaces/%s/serviceaccounts/%s/token",
		namespace,
		serviceAccountName,
	), "-f", tokenRequestFile)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create token: %w", err)
	}

	// Parse the JSON output to extract the token
	var token tokenRequest
	if err := json.Unmarshal(output, &token); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	if token.Status.Token == "" {
		return "", fmt.Errorf("received empty token")
	}

	if err := os.Remove(tokenRequestFile); err != nil {
		return "", fmt.Errorf("failed to remove token request file: %w", err)
	}
	return token.Status.Token, nil
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput(t *testing.T) string {
	t.Log("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	metricsOutput, err := utils.Run(cmd)
	require.NoError(t, err, "Failed to retrieve logs from curl pod")
	require.Contains(t, metricsOutput, "< HTTP/1.1 200 OK")
	return metricsOutput
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}

// TestHeartbeat covers Heartbeat reconciliation: healthy, unhealthy, invalid,
// missing keys, status ranges, and timeouts.
func TestHeartbeat(t *testing.T) {
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

	deployHeartbeatTestServer(t)
	createInitialHealthySecret(t, healthySecretName)
	t.Cleanup(func() { cleanupHeartbeatTestServer(t) })

	// clean isolates subtests the way AfterEach did.
	clean := func(t *testing.T) {
		t.Cleanup(func() { cleanupHeartbeatResources(t, heartbeatName) })
	}

	t.Run("healthy heartbeat", func(t *testing.T) {
		clean(t)
		createHealthyHeartbeat(t, heartbeatName, healthySecretName)
		verifyHeartbeatHealth(t, heartbeatName, true, "Success")
		verifyHeartbeatMessage(t, heartbeatName, heartbeats.ErrEndpointHealthy)
	})

	t.Run("unhealthy endpoints", func(t *testing.T) {
		clean(t)
		createUnhealthyEndpointsSecret(t, unhealthySecretName)
		createUnhealthyHeartbeat(t, heartbeatName, unhealthySecretName)
		verifyHeartbeatHealth(t, heartbeatName+"-unhealthy", false, "Failure")
	})

	t.Run("invalid endpoint URLs", func(t *testing.T) {
		clean(t)
		createInvalidEndpointSecret(t, invalidSecretName)
		verifySecretExists(t, invalidSecretName)
	})

	t.Run("missing secret keys", func(t *testing.T) {
		clean(t)
		createMissingKeysSecret(t, missingKeysSecretName)
		createMissingKeysHeartbeat(t, heartbeatName, missingKeysSecretName)
		verifyHeartbeatHealth(t, heartbeatName, false, "")
		verifyHeartbeatMessageContains(t, heartbeatName, "missing required key")
	})

	t.Run("invalid status code ranges", func(t *testing.T) {
		clean(t)
		createInvalidStatusCodeHeartbeat(t, heartbeatName, healthySecretName)
		verifyHeartbeatExists(t, heartbeatName)
		verifyHeartbeatHealth(t, heartbeatName, false, "")
		verifyHeartbeatMessage(t, heartbeatName, heartbeats.ErrInvalidStatusCodeRange)
	})

	t.Run("multiple status code ranges", func(t *testing.T) {
		clean(t)
		createMultipleRangesSecret(t, multipleRangesSecretName)
		createMultipleRangesHeartbeat(t, heartbeatName, multipleRangesSecretName)
		verifyHeartbeatExists(t, heartbeatName)
		verifyHeartbeatHealth(t, heartbeatName, true, "")
		updateSecretToReturn404(t, multipleRangesSecretName)
		verifyHeartbeatHealth(t, heartbeatName, true, "")
	})

	// The e2e manager runs with --default-timeout=2s --max-retries=1
	// (see config/e2e/manager_args_patch.yaml), so a 5s delay exceeds
	// the timeout without the ~32s production retry budget.
	t.Run("endpoint timeout", func(t *testing.T) {
		clean(t)
		createTimeoutSecret(t, timeoutSecretName)
		createTimeoutHeartbeat(t, heartbeatName, timeoutSecretName)
		verifyHeartbeatExists(t, heartbeatName)
		verifyHeartbeatStatusPopulated(t, heartbeatName)
	})
}

// createInitialHealthySecret creates the initial healthy endpoints secret used by multiple tests.
// This secret is created once in BeforeAll to avoid duplication across tests.
//
// Parameters:
//   - secretName: The name of the secret to create
func createInitialHealthySecret(t *testing.T, secretName string) {
	t.Log("creating a secret with endpoints")
	cmd := exec.Command("kubectl", "create", "secret", "generic", secretName,
		"--from-literal=targetEndpoint="+statusEndpoint(200),
		"--from-literal=healthyEndpoint="+statusEndpoint(200),
		"--from-literal=unhealthyEndpoint="+statusEndpoint(200),
		"-n", namespace)
	_, err := utils.Run(cmd)
	require.NoError(t, err, "Failed to create secret")
}

// deployHeartbeatTestServer deploys a cluster-local HTTP server used by Heartbeat e2e tests.
func deployHeartbeatTestServer(t *testing.T) {
	t.Log("deploying the Heartbeat test HTTP server")
	testServerYAML := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %[1]s
  template:
    metadata:
      labels:
        app: %[1]s
    spec:
      securityContext:
        runAsNonRoot: true
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: http
        image: %[4]s
        imagePullPolicy: IfNotPresent
        command:
        - python
        - -c
        - |
          import re
          import time
          from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

          class Handler(BaseHTTPRequestHandler):
              def handle_request(self):
                  delay_match = re.fullmatch(r"/delay/(\d+)", self.path)
                  status_match = re.fullmatch(r"/status/(\d+)", self.path)
                  if delay_match:
                      time.sleep(int(delay_match.group(1)))
                      status = 200
                  elif status_match:
                      status = int(status_match.group(1))
                  else:
                      status = 200
                  self.send_response(status)
                  self.end_headers()

              do_GET = handle_request
              do_POST = handle_request
              do_PUT = handle_request
              do_PATCH = handle_request

              def log_message(self, format, *args):
                  return

          ThreadingHTTPServer(("0.0.0.0", %[3]d), Handler).serve_forever()
        ports:
        - containerPort: %[3]d
        readinessProbe:
          httpGet:
            path: /status/200
            port: %[3]d
          periodSeconds: 1
        resources:
          requests:
            cpu: 10m
            memory: 32Mi
          limits:
            memory: 64Mi
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop:
            - ALL
          runAsNonRoot: true
          runAsUser: 1000
          seccompProfile:
            type: RuntimeDefault
---
apiVersion: v1
kind: Service
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  selector:
    app: %[1]s
  ports:
  - port: %[3]d
    targetPort: %[3]d
`, heartbeatTestServerName, namespace, heartbeatTestServerPort, heartbeatTestServerImage)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	_, err := utils.RunWithInput(cmd, testServerYAML)
	require.NoError(t, err, "Failed to deploy Heartbeat test server")

	cmd = exec.Command("kubectl", "rollout", "status", "deployment/"+heartbeatTestServerName,
		"-n", namespace, "--timeout=90s")
	_, err = utils.Run(cmd)
	require.NoError(t, err, "Heartbeat test server did not become ready")
}

// cleanupHeartbeatTestServer removes the cluster-local HTTP server used by Heartbeat e2e tests.
func cleanupHeartbeatTestServer(t *testing.T) {
	t.Log("deleting the Heartbeat test HTTP server")
	cmd := exec.Command("kubectl", "delete", "deployment,service", heartbeatTestServerName,
		"-n", namespace, "--ignore-not-found")
	_, err := utils.Run(cmd)
	require.NoError(t, err, "Failed to delete Heartbeat test server")
}

// statusEndpoint returns a cluster-local endpoint that responds with the requested HTTP status.
func statusEndpoint(statusCode int) string {
	return fmt.Sprintf("%s/status/%d", heartbeatTestServerBaseURL(), statusCode)
}

// delayEndpoint returns a cluster-local endpoint that waits before responding.
func delayEndpoint(seconds int) string {
	return fmt.Sprintf("%s/delay/%d", heartbeatTestServerBaseURL(), seconds)
}

// heartbeatTestServerBaseURL returns the in-cluster base URL for the test HTTP server.
func heartbeatTestServerBaseURL() string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
		heartbeatTestServerName,
		namespace,
		heartbeatTestServerPort,
	)
}

// cleanupHeartbeatResources removes Heartbeat resources after each test.
// This ensures that tests don't interfere with each other.
//
// Parameters:
//   - heartbeatName: The base name of the Heartbeat resource to clean up
func cleanupHeartbeatResources(t *testing.T, heartbeatName string) {
	t.Log("deleting the Heartbeat resources")
	cmd := exec.Command("kubectl", "delete", "heartbeat", heartbeatName, "-n", namespace)
	output, err := utils.Run(cmd)
	if err != nil && !strings.Contains(output, "NotFound") {
		require.NoError(t, err, "Failed to delete healthy heartbeat")
	}
	cmd = exec.Command("kubectl", "delete", "heartbeat", heartbeatName+"-unhealthy", "-n", namespace)
	output, err = utils.Run(cmd)
	if err != nil && !strings.Contains(output, "NotFound") {
		require.NoError(t, err, "Failed to delete unhealthy heartbeat")
	}
}

// createHealthyHeartbeat creates a Heartbeat resource that references the healthy endpoints secret.
// It applies the Heartbeat CRD and waits for it to be created.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to create
//   - secretName: The name of the secret containing endpoint configurations
func createHealthyHeartbeat(t *testing.T, heartbeatName, secretName string) {
	t.Log("creating a Heartbeat resource")
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
  interval: 1s
  expectedStatusCodeRanges:
    - min: 200
      max: 299`, heartbeatName, namespace, secretName)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	_, err := utils.RunWithInput(cmd, heartbeatYAML)
	require.NoError(t, err, "Failed to create Heartbeat")
}

// createUnhealthyEndpointsSecret creates a secret with unhealthy endpoint configurations.
// The endpoints are configured to return HTTP 500, which should mark the heartbeat as unhealthy.
//
// Parameters:
//   - secretName: The name of the secret to create
func createUnhealthyEndpointsSecret(t *testing.T, secretName string) {
	t.Log("creating a secret with unhealthy endpoints")
	cmd := exec.Command("kubectl", "create", "secret", "generic", secretName,
		"--from-literal=targetEndpoint="+statusEndpoint(500),
		"--from-literal=healthyEndpoint="+statusEndpoint(200),
		"--from-literal=unhealthyEndpoint="+statusEndpoint(500),
		"-n", namespace)
	_, err := utils.Run(cmd)
	require.NoError(t, err, "Failed to create secret")
}

// createUnhealthyHeartbeat creates a Heartbeat resource that references the unhealthy endpoints secret.
// It applies the Heartbeat CRD and waits for it to be created.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to create
//   - secretName: The name of the secret containing endpoint configurations
func createUnhealthyHeartbeat(t *testing.T, heartbeatName, secretName string) {
	t.Log("creating a Heartbeat resource")
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
  interval: 1s
  expectedStatusCodeRanges:
    - min: 200
      max: 299`, heartbeatName, namespace, secretName)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	_, err := utils.RunWithInput(cmd, heartbeatYAML)
	require.NoError(t, err, "Failed to create Heartbeat")
}

// createInvalidEndpointSecret creates a secret with an invalid endpoint URL.
// This tests the controller's handling of malformed endpoint specifications.
//
// Parameters:
//   - secretName: The name of the secret to create
func createInvalidEndpointSecret(t *testing.T, secretName string) {
	t.Log("creating a secret with an invalid endpoint URL")
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
		base64.StdEncoding.EncodeToString([]byte(statusEndpoint(200))),
		base64.StdEncoding.EncodeToString([]byte(statusEndpoint(200))))

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	_, err := utils.RunWithInput(cmd, secretYAML)
	require.NoError(t, err, "Failed to create secret")
}

// verifySecretExists checks that the specified secret was created successfully.
//
// Parameters:
//   - secretName: The name of the secret to verify
func verifySecretExists(t *testing.T, secretName string) {
	t.Log("verifying the secret was created")
	cmd := exec.Command("kubectl", "get", "secret", secretName, "-n", namespace)
	_, err := utils.Run(cmd)
	require.NoError(t, err, "Secret not found")
}

// createMissingKeysSecret creates a secret with missing required keys.
// This tests the controller's handling of incomplete endpoint configurations.
//
// Parameters:
//   - secretName: The name of the secret to create
func createMissingKeysSecret(t *testing.T, secretName string) {
	t.Log("creating a secret with missing keys")
	var cmd *exec.Cmd
	var err error
	var output string

	// First create the secret
	cmd = exec.Command("kubectl", "create", "secret", "generic", secretName,
		"--from-literal=targetEndpoint="+statusEndpoint(200),
		"--from-literal=healthyEndpoint="+statusEndpoint(200),
		"--from-literal=unhealthyEndpoint="+statusEndpoint(200),
		"-n", namespace)
	_, err = utils.Run(cmd)
	require.NoError(t, err, "Failed to create secret")

	// Then get the secret in YAML format
	cmd = exec.Command("kubectl", "get", "secret", secretName,
		"-n", namespace, "-o", "yaml")
	output, err = utils.Run(cmd)
	require.NoError(t, err)

	// Parse the YAML and modify it to have empty data
	var secretYAML map[string]any
	err = yaml.Unmarshal([]byte(output), &secretYAML)
	require.NoError(t, err)
	secretYAML["data"] = map[string]any{}

	// Convert back to YAML
	modifiedOutput, err := yaml.Marshal(secretYAML)
	require.NoError(t, err)

	// Now replace the secret with empty data
	cmd = exec.Command("kubectl", "replace", "-f", "-")
	_, err = utils.RunWithInput(cmd, string(modifiedOutput))
	require.NoError(t, err, "Failed to update secret")
}

// createMissingKeysHeartbeat creates a Heartbeat resource that references the missing keys secret.
// It applies the Heartbeat CRD and waits for it to be created.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to create
//   - secretName: The name of the secret containing endpoint configurations
func createMissingKeysHeartbeat(t *testing.T, heartbeatName, secretName string) {
	t.Log("creating a Heartbeat resource")
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
  interval: 1s
  expectedStatusCodeRanges:
    - min: 200
      max: 299`, heartbeatName, namespace, secretName)
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	_, err := utils.RunWithInput(cmd, heartbeatYAML)
	require.NoError(t, err, "Failed to create Heartbeat resource")
}

// createInvalidStatusCodeHeartbeat creates a Heartbeat resource with invalid status code ranges.
// This tests the controller's handling of malformed status code specifications.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to create
//   - secretName: The name of the secret containing endpoint configurations
func createInvalidStatusCodeHeartbeat(t *testing.T, heartbeatName, secretName string) {
	t.Log("creating a Heartbeat with invalid status code ranges")
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
  interval: 1s
  expectedStatusCodeRanges:
    - min: 300
      max: 200`, heartbeatName, namespace, secretName)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	_, err := utils.RunWithInput(cmd, heartbeatYAML)
	require.NoError(t, err, "Failed to create Heartbeat")
}

// createMultipleRangesSecret creates a secret with multiple status code range configurations.
// This tests the controller's handling of complex status code specifications.
//
// Parameters:
//   - secretName: The name of the secret to create
func createMultipleRangesSecret(t *testing.T, secretName string) {
	t.Log("creating a secret with multiple status code ranges")
	cmd := exec.Command("kubectl", "create", "secret", "generic", secretName,
		"--from-literal=targetEndpoint="+statusEndpoint(200),
		"--from-literal=healthyEndpoint="+statusEndpoint(200),
		"--from-literal=unhealthyEndpoint="+statusEndpoint(200),
		"-n", namespace)
	_, err := utils.Run(cmd)
	require.NoError(t, err, "Failed to create secret")
}

// createMultipleRangesHeartbeat creates a Heartbeat resource that references the multiple ranges secret.
// It applies the Heartbeat CRD and waits for it to be created.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to create
//   - secretName: The name of the secret containing endpoint configurations
func createMultipleRangesHeartbeat(t *testing.T, heartbeatName, secretName string) {
	t.Log("creating a Heartbeat resource")
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
  interval: 1s
  expectedStatusCodeRanges:
    - min: 200
      max: 299
    - min: 404
      max: 404`, heartbeatName, namespace, secretName)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	_, err := utils.RunWithInput(cmd, heartbeatYAML)
	require.NoError(t, err, "Failed to create Heartbeat")
}

// createTimeoutSecret creates a secret with timeout configurations.
// This tests the controller's handling of timeout settings.
//
// Parameters:
//   - secretName: The name of the secret to create
func createTimeoutSecret(t *testing.T, secretName string) {
	t.Log("creating a secret with a timeout endpoint")
	cmd := exec.Command("kubectl", "create", "secret", "generic", secretName,
		"--from-literal=targetEndpoint="+delayEndpoint(5),
		"--from-literal=healthyEndpoint="+statusEndpoint(200),
		"--from-literal=unhealthyEndpoint="+statusEndpoint(200),
		"-n", namespace)
	_, err := utils.Run(cmd)
	require.NoError(t, err, "Failed to create secret")
}

// createTimeoutHeartbeat creates a Heartbeat resource that references the timeout secret.
// It applies the Heartbeat CRD and waits for it to be created.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to create
//   - secretName: The name of the secret containing endpoint configurations
func createTimeoutHeartbeat(t *testing.T, heartbeatName, secretName string) {
	t.Log("creating a Heartbeat resource")
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
  interval: 1s
  expectedStatusCodeRanges:
    - min: 200
      max: 299`, heartbeatName, namespace, secretName)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	_, err := utils.RunWithInput(cmd, heartbeatYAML)
	require.NoError(t, err, "Failed to create Heartbeat")
}

// verifyHeartbeatExists checks that the specified Heartbeat resource exists.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to verify
func verifyHeartbeatExists(t *testing.T, heartbeatName string) {
	t.Log("waiting for the Heartbeat resource to be ready")
	require.Eventually(t, func() bool {
		cmd := exec.Command("kubectl", "get", "heartbeat", heartbeatName, "-n", namespace)
		_, err := utils.Run(cmd)
		return err == nil
	}, 10*time.Second, time.Second, "Heartbeat resource not ready")
}

// verifyHeartbeatHealth verifies that the Heartbeat resource has the expected health status and report status.
// It polls the Heartbeat status until the expected values are reached or timeout occurs.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to verify
//   - expectedHealthy: The expected health status (true for healthy, false for unhealthy)
//   - expectedReportStatus: The expected report status (e.g., "Success", "Failure")
func verifyHeartbeatHealth(t *testing.T, heartbeatName string, expectedHealthy bool, expectedReportStatus string) {
	t.Log("verifying the Heartbeat status")
	require.Eventually(t, func() bool {
		cmd := exec.Command("kubectl", "get", "heartbeat", heartbeatName,
			"-o", "jsonpath={.status.healthy}",
			"-n", namespace)
		output, err := utils.Run(cmd)
		if err != nil || (output == "true") != expectedHealthy {
			return false
		}
		if expectedReportStatus != "" {
			cmd = exec.Command("kubectl", "get", "heartbeat", heartbeatName,
				"-o", "jsonpath={.status.reportStatus}",
				"-n", namespace)
			reportStatus, err := utils.Run(cmd)
			if err != nil || reportStatus != expectedReportStatus {
				return false
			}
		}
		return true
	}, 60*time.Second, time.Second, "Heartbeat health status mismatch")
}

// verifyHeartbeatMessage verifies that the Heartbeat resource has the expected status message.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to verify
//   - expectedMessage: The expected status message
func verifyHeartbeatMessage(t *testing.T, heartbeatName, expectedMessage string) {
	t.Log("verifying the Heartbeat message contains the correct status code")
	cmd := exec.Command("kubectl", "get", "heartbeat", heartbeatName,
		"-o", "jsonpath={.status.message}",
		"-n", namespace)
	output, err := utils.Run(cmd)
	require.NoError(t, err)
	require.Equal(t, expectedMessage, output)
}

// verifyHeartbeatMessageContains verifies that the Heartbeat resource's status message contains the expected substring.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to verify
//   - expectedSubstring: The expected substring in the status message
func verifyHeartbeatMessageContains(t *testing.T, heartbeatName, expectedSubstring string) {
	t.Log("verifying the Heartbeat message indicates missing keys")
	require.Eventually(t, func() bool {
		cmd := exec.Command("kubectl", "get", "heartbeat", heartbeatName,
			"-o", "jsonpath={.status.message}",
			"-n", namespace)
		output, err := utils.Run(cmd)
		return err == nil && strings.Contains(output, expectedSubstring)
	}, 15*time.Second, time.Second, "Heartbeat message missing expected substring")
}

// updateSecretToReturn404 updates the secret to return a 404 status code.
// This tests the controller's handling of different status codes within the valid range.
//
// Parameters:
//   - secretName: The name of the secret to update
func updateSecretToReturn404(t *testing.T, secretName string) {
	t.Log("updating the secret to return 404 status code")
	data := map[string]string{
		"targetEndpoint":    base64.StdEncoding.EncodeToString([]byte(statusEndpoint(404))),
		"healthyEndpoint":   base64.StdEncoding.EncodeToString([]byte(statusEndpoint(200))),
		"unhealthyEndpoint": base64.StdEncoding.EncodeToString([]byte(statusEndpoint(200))),
	}
	patch, err := json.Marshal(map[string]any{"data": data})
	require.NoError(t, err, "Failed to build secret patch")

	cmd := exec.Command("kubectl", "patch", "secret", secretName,
		"--type=merge", "-p", string(patch), "-n", namespace)
	_, err = utils.Run(cmd)
	require.NoError(t, err, "Failed to update secret")
}

// verifyHeartbeatStatusPopulated verifies that the Heartbeat resource's status message is populated.
// This is used for tests where we expect the status to be set but don't care about the specific value.
//
// Parameters:
//   - heartbeatName: The name of the Heartbeat resource to verify
func verifyHeartbeatStatusPopulated(t *testing.T, heartbeatName string) {
	t.Log("waiting for the Heartbeat status to be populated")
	require.Eventually(t, func() bool {
		cmd := exec.Command("kubectl", "get", "heartbeat", heartbeatName,
			"-o", "jsonpath={.status.message}",
			"-n", namespace)
		output, err := utils.Run(cmd)
		return err == nil && output != ""
	}, 30*time.Second, time.Second, "Heartbeat status message should be populated")
}
