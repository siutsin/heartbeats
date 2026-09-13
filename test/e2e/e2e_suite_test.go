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
// TestMain owns the cluster lifecycle: fresh Kind cluster, operator image,
// deploy, run, teardown.
package e2e

import (
	"log"
	"os"
	"os/exec"
	"testing"

	"github.com/siutsin/heartbeats/test/utils"
)

// projectImage is the operator image tag for e2e tests.
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

// TestMain sets up the cluster and operator once, runs all tests, then tears down.
func TestMain(m *testing.M) {
	if err := setupKindCluster(); err != nil {
		log.Fatalf("setup kind cluster: %v", err)
	}
	if err := buildAndLoadOperatorImage(); err != nil {
		log.Fatalf("build operator image: %v", err)
	}
	if err := deployOperator(); err != nil {
		log.Fatalf("deploy operator: %v", err)
	}
	code := m.Run()
	if err := teardownKindCluster(); err != nil {
		log.Printf("teardown kind cluster: %v", err)
	}
	os.Exit(code)
}

// setupKindCluster creates a fresh Kind cluster for testing.
// It first deletes any existing cluster to ensure a clean environment.
func setupKindCluster() error {
	if os.Getenv("CLUSTER_BACKEND") == "apple" {
		log.Println("using the Apple Container cluster prepared by the Makefile")
		return nil
	}
	log.Println("deleting any existing Kind cluster")
	cmd := exec.Command("kind", "delete", "cluster")
	if _, err := utils.Run(cmd); err != nil {
		log.Println("no existing cluster to delete")
	}
	log.Println("creating a new Kind cluster")
	cmd = exec.Command("kind", "create", "cluster", "--config", "test/e2e/kind-config.yaml")
	_, err := utils.Run(cmd)
	return err
}

// teardownKindCluster removes the Kind cluster to free up system resources.
func teardownKindCluster() error {
	if os.Getenv("CLUSTER_BACKEND") == "apple" {
		log.Println("leaving the Apple Container cluster running")
		return nil
	}
	log.Println("deleting Kind cluster")
	cmd := exec.Command("kind", "delete", "cluster")
	_, err := utils.Run(cmd)
	return err
}

// buildAndLoadOperatorImage builds the operator image and loads it into the Kind cluster.
// The build is skipped when the image already exists locally (e.g. prebuilt
// by CI with layer caching), so the image is built once, not twice.
func buildAndLoadOperatorImage() error {
	if !localImageExists(projectImage) {
		log.Println("building the manager(Operator) image")
		cmd := exec.Command("make", "docker-build", "IMG="+projectImage)
		if _, err := utils.Run(cmd); err != nil {
			return err
		}
	} else {
		log.Println("reusing the existing manager(Operator) image")
	}
	log.Println("loading the manager(Operator) image on Kind")
	return utils.LoadImageToKindClusterWithName(projectImage)
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
func deployOperator() error {
	log.Println("installing CRDs")
	cmd := exec.Command("make", "install")
	if _, err := utils.Run(cmd); err != nil {
		return err
	}
	log.Println("deploying the controller-manager")
	cmd = exec.Command("make", "deploy-test", "IMG="+projectImage)
	if _, err := utils.Run(cmd); err != nil {
		return err
	}
	log.Println("waiting for the controller-manager to be ready")
	cmd = exec.Command("kubectl", "wait",
		"--for=condition=Available",
		"deployment",
		"-n", namespace,
		"heartbeats-operator-controller-manager",
		"--timeout=2m")
	_, err := utils.Run(cmd)
	return err
}
