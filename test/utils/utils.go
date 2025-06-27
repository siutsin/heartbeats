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

// Package utils provides utility functions for e2e testing.
// This package contains helper functions for executing commands, managing Kubernetes resources,
// and handling common testing operations like installing/uninstalling operators and checking CRDs.
package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	ginkgo "github.com/onsi/ginkgo/v2" //nolint:revive
)

// Constants for operator versions and URLs
const (
	// prometheusOperatorVersion specifies the version of Prometheus Operator to install
	prometheusOperatorVersion = "v0.77.1"
	// prometheusOperatorURL is the base URL for downloading Prometheus Operator bundle
	prometheusOperatorURL = "https://github.com/prometheus-operator/prometheus-operator/" +
		"releases/download/%s/bundle.yaml"

	// certmanagerVersion specifies the version of cert-manager to install
	certmanagerVersion = "v1.16.3"
	// certmanagerURLTmpl is the template URL for downloading cert-manager bundle
	certmanagerURLTmpl = "https://github.com/cert-manager/cert-manager/releases/download/%s/cert-manager.yaml"
)

// warnError logs a warning message when an error occurs during cleanup operations.
// This function is used for non-critical errors that shouldn't fail the test.
//
// Parameters:
//   - err: The error to log as a warning
func warnError(err error) {
	_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "warning: %v\n", err)
}

// Run executes the provided command within the project context.
// It sets up the working directory, environment variables, and captures both stdout and stderr.
// The function logs the command being executed and returns the combined output.
//
// Parameters:
//   - cmd: The command to execute
//
// Returns:
//   - string: The combined stdout and stderr output
//   - error: Any error that occurred during command execution
func Run(cmd *exec.Cmd) (string, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "chdir dir: %s\n", err)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "running: %s\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s failed with error: (%v) %s", command, err, string(output))
	}

	return string(output), nil
}

// RunWithInput executes the provided command with input from stdin.
// This is useful for commands like `kubectl apply -f -` that expect YAML input.
//
// Parameters:
//   - cmd: The command to execute
//   - input: The input string to provide via stdin
//
// Returns:
//   - string: The combined stdout and stderr output
//   - error: Any error that occurred during command execution
func RunWithInput(cmd *exec.Cmd, input string) (string, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "chdir dir: %s\n", err)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "running: %s\n", command)

	// Set up pipes for stdin, stdout, and stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdin pipe: %v", err)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Start the command
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start command: %v", err)
	}

	// Write input to stdin
	if _, err := stdin.Write([]byte(input)); err != nil {
		return "", fmt.Errorf("failed to write to stdin: %v", err)
	}
	if err := stdin.Close(); err != nil {
		return "", fmt.Errorf("failed to close stdin: %v", err)
	}

	// Wait for command to complete
	err = cmd.Wait()
	if err != nil {
		combinedOutput := stdout.String() + stderr.String()
		return combinedOutput, fmt.Errorf("%s failed with error: (%v) %s", command, err, combinedOutput)
	}

	// Return combined output
	return stdout.String() + stderr.String(), nil
}

// InstallPrometheusOperator installs the Prometheus Operator to be used for exporting metrics.
// It downloads and applies the operator bundle from the official GitHub releases.
//
// Returns:
//   - error: Any error that occurred during installation
func InstallPrometheusOperator() error {
	url := fmt.Sprintf(prometheusOperatorURL, prometheusOperatorVersion)
	cmd := exec.Command("kubectl", "create", "-f", url)
	_, err := Run(cmd)
	return err
}

// UninstallPrometheusOperator uninstalls the Prometheus Operator.
// It removes the operator bundle that was previously installed.
// Errors during uninstallation are logged as warnings since they're not critical.
func UninstallPrometheusOperator() {
	url := fmt.Sprintf(prometheusOperatorURL, prometheusOperatorVersion)
	cmd := exec.Command("kubectl", "delete", "-f", url)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// IsPrometheusCRDsInstalled checks if any Prometheus CRDs are installed
// by verifying the existence of key CRDs related to Prometheus.
// This function is useful for determining if Prometheus Operator is already installed.
//
// Returns:
//   - bool: True if any Prometheus CRDs are found, false otherwise
func IsPrometheusCRDsInstalled() bool {
	// List of common Prometheus CRDs to check for
	prometheusCRDs := []string{
		"prometheuses.monitoring.coreos.com",
		"prometheusrules.monitoring.coreos.com",
		"prometheusagents.monitoring.coreos.com",
	}

	cmd := exec.Command("kubectl", "get", "crds", "-o", "custom-columns=NAME:.metadata.name")
	output, err := Run(cmd)
	if err != nil {
		return false
	}
	crdList := GetNonEmptyLines(output)
	for _, crd := range prometheusCRDs {
		for _, line := range crdList {
			if strings.Contains(line, crd) {
				return true
			}
		}
	}

	return false
}

// UninstallCertManager uninstalls the cert-manager bundle.
// It removes the cert-manager that was previously installed.
// Errors during uninstallation are logged as warnings since they're not critical.
func UninstallCertManager() {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "delete", "-f", url)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// InstallCertManager installs the cert-manager bundle.
// It downloads and applies the cert-manager bundle, then waits for the webhook to be ready.
// The webhook readiness check is important because cert-manager may be re-installed
// after uninstalling on a cluster.
//
// Returns:
//   - error: Any error that occurred during installation
func InstallCertManager() error {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "apply", "-f", url)
	if _, err := Run(cmd); err != nil {
		return err
	}

	// Wait for cert-manager-webhook to be ready, which can take time if cert-manager
	// was re-installed after uninstalling on a cluster.
	cmd = exec.Command("kubectl", "wait", "deployment.apps/cert-manager-webhook",
		"--for", "condition=Available",
		"--namespace", "cert-manager",
		"--timeout", "5m",
	)

	_, err := Run(cmd)
	return err
}

// IsCertManagerCRDsInstalled checks if any Cert Manager CRDs are installed
// by verifying the existence of key CRDs related to Cert Manager.
// This function is useful for determining if cert-manager is already installed.
//
// Returns:
//   - bool: True if any Cert Manager CRDs are found, false otherwise
func IsCertManagerCRDsInstalled() bool {
	// List of common Cert Manager CRDs to check for
	certManagerCRDs := []string{
		"certificates.cert-manager.io",
		"issuers.cert-manager.io",
		"clusterissuers.cert-manager.io",
		"certificaterequests.cert-manager.io",
		"orders.acme.cert-manager.io",
		"challenges.acme.cert-manager.io",
	}

	// Execute the kubectl command to get all CRDs
	cmd := exec.Command("kubectl", "get", "crds")
	output, err := Run(cmd)
	if err != nil {
		return false
	}

	// Check if any of the Cert Manager CRDs are present
	crdList := GetNonEmptyLines(output)
	for _, crd := range certManagerCRDs {
		for _, line := range crdList {
			if strings.Contains(line, crd) {
				return true
			}
		}
	}

	return false
}

// LoadImageToKindClusterWithName loads a local docker image to the kind cluster.
// This function is useful for testing with custom images that aren't available in public registries.
// The cluster name can be overridden by setting the KIND_CLUSTER environment variable.
//
// Parameters:
//   - name: The name of the docker image to load
//
// Returns:
//   - error: Any error that occurred during image loading
func LoadImageToKindClusterWithName(name string) error {
	cluster := "kind"
	if v, ok := os.LookupEnv("KIND_CLUSTER"); ok {
		cluster = v
	}
	kindOptions := []string{"load", "docker-image", name, "--name", cluster}
	cmd := exec.Command("kind", kindOptions...)
	_, err := Run(cmd)
	return err
}

// GetNonEmptyLines converts given command output string into individual objects
// according to line breakers, and ignores the empty elements in it.
// This function is useful for parsing command output that contains multiple lines.
//
// Parameters:
//   - output: The command output string to parse
//
// Returns:
//   - []string: A slice of non-empty lines from the output
func GetNonEmptyLines(output string) []string {
	var res []string
	elements := strings.Split(output, "\n")
	for _, element := range elements {
		if element != "" {
			res = append(res, element)
		}
	}

	return res
}

// GetProjectDir will return the directory where the project is located.
// It handles the case where the current working directory might be in a subdirectory
// like test/e2e and adjusts the path accordingly.
//
// Returns:
//   - string: The project root directory path
//   - error: Any error that occurred while getting the working directory
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, err
	}
	wd = strings.ReplaceAll(wd, "/test/e2e", "")
	return wd, nil
}

// UncommentCode searches for target in the file and removes the comment prefix
// of the target content. The target content may span multiple lines.
// This function is useful for programmatically enabling features in configuration files.
//
// Parameters:
//   - filename: The path to the file to modify
//   - target: The target content to uncomment
//   - prefix: The comment prefix to remove (e.g., "// " or "# ")
//
// Returns:
//   - error: Any error that occurred during file modification
func UncommentCode(filename, target, prefix string) error {
	// false positive
	// nolint:gosec
	content, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	strContent := string(content)

	idx := strings.Index(strContent, target)
	if idx < 0 {
		return fmt.Errorf("unable to find the code %s to be uncomment", target)
	}

	out := new(bytes.Buffer)
	_, err = out.Write(content[:idx])
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(bytes.NewBufferString(target))
	if !scanner.Scan() {
		return nil
	}
	for {
		_, err := out.WriteString(strings.TrimPrefix(scanner.Text(), prefix))
		if err != nil {
			return err
		}
		// Avoid writing a newline in case the previous line was the last in target.
		if !scanner.Scan() {
			break
		}
		if _, err := out.WriteString("\n"); err != nil {
			return err
		}
	}

	_, err = out.Write(content[idx+len(target):])
	if err != nil {
		return err
	}
	// false positive
	// nolint:gosec
	return os.WriteFile(filename, out.Bytes(), 0644)
}
