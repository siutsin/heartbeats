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
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// logf writes formatted output to stdout without changing test control flow.
func logf(format string, args ...any) {
	fmt.Printf(format, args...)
}

// warnError logs a warning message when an error occurs during cleanup operations.
// This function is used for non-critical errors that shouldn't fail the test.
//
// Parameters:
//   - err: The error to log as a warning
func warnError(err error) {
	logf("warning: %v\n", err)
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
	dir, err := GetProjectDir()
	if err != nil {
		return "", err
	}
	cmd.Dir = dir

	if chdirErr := os.Chdir(cmd.Dir); chdirErr != nil {
		logf("chdir dir: %s\n", chdirErr)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	logf("running: %s\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s failed with error: (%w) %s", command, err, string(output))
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
	dir, err := GetProjectDir()
	if err != nil {
		return "", err
	}
	cmd.Dir = dir

	if chdirErr := os.Chdir(cmd.Dir); chdirErr != nil {
		logf("chdir dir: %s\n", chdirErr)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	logf("running: %s\n", command)

	// Set up pipes for stdin, stdout, and stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Start the command
	if startErr := cmd.Start(); startErr != nil {
		return "", fmt.Errorf("failed to start command: %w", startErr)
	}

	// Write input to stdin
	if _, writeErr := stdin.Write([]byte(input)); writeErr != nil {
		return "", fmt.Errorf("failed to write to stdin: %w", writeErr)
	}
	if closeErr := stdin.Close(); closeErr != nil {
		return "", fmt.Errorf("failed to close stdin: %w", closeErr)
	}

	// Wait for command to complete
	err = cmd.Wait()
	if err != nil {
		combinedOutput := stdout.String() + stderr.String()
		return combinedOutput, fmt.Errorf("%s failed with error: (%w) %s", command, err, combinedOutput)
	}

	// Return combined output
	return stdout.String() + stderr.String(), nil
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

	// Apple Container clusters load images with the container CLI.
	if os.Getenv("CLUSTER_BACKEND") == "apple" {
		cmd := exec.Command("container", "k8s", "load-image", "--name", cluster, name)
		if _, err := Run(cmd); err != nil {
			return err
		}
		return appleImageTag(cluster, name)
	}

	// When using podman, export image to tar and load into kind
	if provider, ok := os.LookupEnv("KIND_EXPERIMENTAL_PROVIDER"); ok && provider == "podman" {
		tmpFile, err := os.CreateTemp("", "kind-image-*.tar")
		if err != nil {
			return err
		}

		defer func() {
			if removeErr := os.Remove(tmpFile.Name()); removeErr != nil {
				warnError(removeErr)
			}
		}()
		if closeErr := tmpFile.Close(); closeErr != nil {
			return closeErr
		}

		// Ensure we have both localhost/ prefixed and non-prefixed tags
		podmanImage := name
		if !strings.HasPrefix(name, "localhost/") {
			podmanImage = "localhost/" + name
		}

		// Tag image without localhost/ prefix
		if _, runErr := Run(exec.Command("podman", "tag", podmanImage, name)); runErr != nil {
			return runErr
		}

		// Save the non-prefixed image to tar
		if _, runErr := Run(exec.Command("podman", "save", "-o", tmpFile.Name(), name)); runErr != nil {
			return runErr
		}

		// Copy tar into kind container
		destPath := "/tmp/" + filepath.Base(tmpFile.Name())
		containerName := cluster + "-control-plane"
		if _, runErr := Run(exec.Command("podman", "cp", tmpFile.Name(), containerName+":"+destPath)); runErr != nil {
			return runErr
		}

		// Import image into containerd using ctr
		_, err = Run(exec.Command("podman", "exec", containerName, "ctr", "-n", "k8s.io", "images", "import", destPath))
		if err != nil {
			return err
		}

		// Tag the image to remove localhost/ prefix (containerd imports with localhost/ but k8s expects docker.io/library/)
		localImage := "localhost/" + name
		dockerImage := "docker.io/library/" + name
		cmd := exec.Command("podman", "exec", containerName, "ctr", "-n", "k8s.io", "images", "tag", localImage, dockerImage)
		if _, err := Run(cmd); err != nil {
			return err
		}

		// Clean up the tar file inside the container
		if _, err := Run(exec.Command("podman", "exec", containerName, "rm", destPath)); err != nil {
			warnError(err)
		}
		return nil
	}

	// For docker, use direct image loading
	kindOptions := []string{"load", "docker-image", name, "--name", cluster}
	cmd := exec.Command("kind", kindOptions...)
	_, err := Run(cmd)
	return err
}

// appleImageTag aligns the in-node image reference with the name kubelet
// looks up, so pods start without a registry. load-image stores single-name
// images short and prefixes multi-part names with docker.io, while kubelet
// wants docker.io/library for single names and docker.io for multi-part ones.
// The node name matches the cluster name for single-node clusters.
//
// Parameters:
//   - cluster: The Apple Container cluster (and node) name
//   - name: The image name to qualify
//
// Returns:
//   - error: Any error that occurred during tagging
func appleImageTag(cluster, name string) error {
	nodeName, kubeName := name, name
	host, _, found := strings.Cut(name, "/")
	if !found {
		kubeName = "docker.io/library/" + name
	} else if !strings.Contains(host, ".") && !strings.Contains(host, ":") && host != "localhost" {
		nodeName = "docker.io/" + name
		kubeName = nodeName
	}
	if nodeName == kubeName {
		return nil
	}
	cmd := exec.Command("container", "exec", cluster,
		"ctr", "-n", "k8s.io", "images", "tag", nodeName, kubeName)
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
