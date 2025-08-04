// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e_test

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

// startUMHCoreWithMockAPI starts a UMH Core container configured to use the mock API server
func startUMHCoreWithMockAPI(mockServer *MockAPIServer) (string, int) {
	// Generate unique container name
	suffix := make([]byte, 4)
	rand.Read(suffix)
	containerName := fmt.Sprintf("umh-core-e2e-%s", hex.EncodeToString(suffix))

	imageName := getE2EImageName()

	dataDir := createTempConfigDir(containerName)

	// Generate a test auth token
	authToken := generateTestAuthToken()

	fmt.Printf("Starting container %s with API URL %s\n",
		containerName, mockServer.GetURL())

	// Find two open ports for metrics and graphql using :0
	metricsPort := getAvailablePort()
	graphQLPort := getAvailablePort()

	// Start the container with environment variables pointing to our mock server
	cmd := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"--restart", "unless-stopped",
		"-v", fmt.Sprintf("%s:/data", dataDir),
		"-e", fmt.Sprintf("AUTH_TOKEN=%s", authToken),
		"-e", fmt.Sprintf("API_URL=%s", mockServer.GetURL()),
		"-e", "LOCATION_0=E2E-Test-Plant",
		"-e", "LOCATION_1=Test-Line",
		"-e", "ALLOW_INSECURE_TLS=true", // Since we're using HTTP for testing
		"-p", fmt.Sprintf("%d:8080", metricsPort), // Bind to random port for metrics
		"-p", fmt.Sprintf("%d:8090", graphQLPort), // Bind to random port for GraphQL
		imageName,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Print debug info before panicking
		fmt.Printf("Docker command failed: %s\n", cmd.String())
		fmt.Printf("Output: %s\n", string(output))
		panic(fmt.Sprintf("Failed to start container: %v", err))
	}

	fmt.Printf("Container %s started successfully\n", containerName)
	return containerName, metricsPort
}

// createTempConfigDir creates a temporary directory for the test configuration
func createTempConfigDir(containerName string) string {
	tempDir := os.TempDir()
	configDir := filepath.Join(tempDir, fmt.Sprintf("umh-e2e-%s", containerName))

	err := os.MkdirAll(configDir, 0755)
	if err != nil {
		panic(fmt.Sprintf("Failed to create config directory: %v", err))
	}

	return configDir
}

// generateTestAuthToken generates a test authentication token
func generateTestAuthToken() string {
	// For testing, we just generate a simple token
	tokenBytes := make([]byte, 16)
	rand.Read(tokenBytes)
	return hex.EncodeToString(tokenBytes)
}

// getAvailablePort returns an port that is not used by the OS
func getAvailablePort() int {
	// Find an available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(fmt.Sprintf("Failed to find available port: %v", err))
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}

// getE2EImageName returns the image name to use for E2E testing
func getE2EImageName() string {
	// Check if there's an environment variable set
	if imageName := os.Getenv("UMH_CORE_E2E_IMAGE"); imageName != "" {
		return imageName
	}

	// Default to the latest tag - in a real setup you'd build this as part of the test
	return "umh-core:latest"
}

// isContainerHealthy checks if the container is healthy by testing the metrics endpoint
func isContainerHealthy(metricsPort int) bool {
	// Test the metrics endpoint
	metricsURL := fmt.Sprintf("http://localhost:%d/metrics", metricsPort)
	resp, err := http.Get(metricsURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// stopAndRemoveContainer stops and removes the test container
func stopAndRemoveContainer(containerName string) {
	fmt.Printf("Stopping and removing container %s\n", containerName)

	// Stop the container
	stopCmd := exec.Command("docker", "stop", containerName)
	stopCmd.Run() // Ignore errors

	// Remove the container
	rmCmd := exec.Command("docker", "rm", containerName)
	rmCmd.Run() // Ignore errors

	fmt.Printf("Container %s cleaned up\n", containerName)
}

// getContainerLogs gets the logs from the container for debugging
func getContainerLogs(containerName string) string {
	cmd := exec.Command("docker", "logs", containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Failed to get logs: %v", err)
	}
	return string(output)
}

// printContainerDebugInfo prints debugging information about the container
func printContainerDebugInfo(containerName string) {
	fmt.Printf("=== Debug Info for Container %s ===\n", containerName)

	// Container status
	statusCmd := exec.Command("docker", "ps", "-a", "--filter", fmt.Sprintf("name=%s", containerName))
	if output, err := statusCmd.Output(); err == nil {
		fmt.Printf("Container status:\n%s\n", string(output))
	}

	// Container logs
	fmt.Printf("Container logs:\n%s\n", getContainerLogs(containerName))

	fmt.Printf("=== End Debug Info ===\n")
}
