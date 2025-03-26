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

package integration_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	. "github.com/onsi/gomega"
)

const (
	containerBaseName = "umh-core"
	imageName         = "umh-core:latest"
)

var (
	// containerName is a unique name for this test run
	containerName string
	containerOnce sync.Once

	// Store port mappings for each container
	metricsPort int
	goldenPort  int
	portMu      sync.Mutex

	// Test-specific data directory
	dataDir     string
	dataDirOnce sync.Once
)

// getContainerName returns a unique container name for this test run
func getContainerName() string {
	containerOnce.Do(func() {
		// Generate a random 8-character suffix
		suffix := make([]byte, 4)
		_, err := rand.Read(suffix)
		if err != nil {
			// Fallback to timestamp if random fails
			containerName = fmt.Sprintf("%s-%d", containerBaseName, os.Getpid())
			return
		}
		containerName = fmt.Sprintf("%s-%s", containerBaseName, hex.EncodeToString(suffix))
	})
	return containerName
}

// getTestDataDir returns a unique data directory for this test run
func getTestDataDir() string {
	dataDirOnce.Do(func() {
		// Use the same suffix as the container name for consistency
		suffix := strings.TrimPrefix(getContainerName(), containerBaseName+"-")
		dataDir = filepath.Join(GetCurrentDir(), "data")
		dataDir = filepath.Join(dataDir, "data-"+suffix)
		// Create the directory
		err := os.MkdirAll(dataDir, 0o755)
		if err != nil {
			// Fallback to standard data dir
			dataDir = filepath.Join(GetCurrentDir(), "data")
		}
	})
	return dataDir
}

// GetMetricsURL returns the URL for the metrics endpoint, using the container's IP if possible
func GetMetricsURL() string {
	portMu.Lock()
	port := metricsPort
	portMu.Unlock()
	if port == 0 {
		port = 8080 // Fallback to default port
	}
	return fmt.Sprintf("http://host.docker.internal:%d/metrics", port)
}

// GetGoldenServiceURL returns the URL for the golden service
func GetGoldenServiceURL() string {
	portMu.Lock()
	port := goldenPort
	portMu.Unlock()
	if port == 0 {
		port = 8082 // Fallback to default port
	}
	return fmt.Sprintf("http://host.docker.internal:%d", port)
}

// BuildAndRunContainer rebuilds your Docker image, starts the container, etc.
func BuildAndRunContainer(configFilePath string, memory string) error {
	// Get the unique container name for this test run
	containerName := getContainerName()

	// Generate random ports to avoid conflicts
	// Use PID to create somewhat unique ports, but with a limited range to avoid system port limits
	metricsPrt := 8081 + (os.Getpid() % 1000) // Base port 8081 + offset based on PID
	goldenPrt := 8082 + (os.Getpid() % 1000)  // Base port 8082 + offset based on PID

	// Store these ports for later use
	portMu.Lock()
	metricsPort = metricsPrt
	goldenPort = goldenPrt
	portMu.Unlock()

	// 1. Stop/Remove the old container if any
	runDockerCommand("rm", "-f", containerName) // ignoring error
	runDockerCommand("stop", containerName)     // ignoring error

	// 2. Build image
	coreDir := filepath.Dir(GetCurrentDir())
	dockerfilePath := filepath.Join(coreDir, "Dockerfile")
	out, err := runDockerCommand("build", "-t", imageName, "-f", dockerfilePath, coreDir)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Docker build failed: %s", out))

	// 3. Run container
	out, err = runDockerCommand(
		"run", "-d",
		"--name", containerName,
		"--cpus=1",
		"--memory", memory,
		"-v", fmt.Sprintf("%s:/data", getTestDataDir()),
		"-e", "LOGGING_LEVEL=debug",
		"-p", fmt.Sprintf("%d:8080", metricsPrt),
		"-p", fmt.Sprintf("%d:8082", goldenPrt),
		imageName,
	)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Could not run container: %s", out))

	// 4. Wait for the container to be healthy
	return waitForMetrics()
}

// cleanupTestData removes the test-specific data directory
func cleanupTestData() {
	if dataDir != "" {
		// Just attempt to remove it, ignoring errors
		os.RemoveAll(dataDir)
	}
}

// getContainerIP returns the IP address of a running container
func getContainerIP(container string) (string, error) {
	// We're using runDockerCommand which already handles docker/podman selection
	out, err := runDockerCommand("inspect", "--format", "{{.NetworkSettings.IPAddress}}", container)
	if err != nil {
		return "", fmt.Errorf("failed to get container IP: %w", err)
	}

	// Trim whitespace
	ip := strings.TrimSpace(out)
	return ip, nil
}

// printContainerLogs prints the logs from the container
func printContainerLogs() {
	containerName := getContainerName()
	out, err := runDockerCommand("logs", containerName)
	if err != nil {
		fmt.Printf("Failed to get container logs: %v\n", err)
		return
	}
	fmt.Printf("Container logs:\n%s\n", out)
}

// StopContainer stops and removes your container
func StopContainer() {
	containerName := getContainerName()
	// First stop the container
	runDockerCommand("stop", containerName)
	// Then remove it
	runDockerCommand("rm", "-f", containerName)

	// Clean up test data
	cleanupTestData()
}

func runDockerCommand(args ...string) (string, error) {
	// Check if we use docker or podman
	dockerCmd := "docker"
	if _, err := exec.LookPath("podman"); err == nil {
		dockerCmd = "podman"
	}
	cmd := exec.Command(dockerCmd, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// GetCurrentDir returns the directory of this test file (or your project root).
// Adjust if you need something else.
func GetCurrentDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return strings.TrimSpace(wd)
}

// writeConfigFile writes the given YAML content to ./data/config.yaml so the container will read it.
func writeConfigFile(yamlContent string) error {
	dataDir := getTestDataDir()
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("failed to create data dir: %w", err)
	}
	configPath := filepath.Join(dataDir, "config.yaml")
	return os.WriteFile(configPath, []byte(yamlContent), 0o644)
}

// printContainerDebugInfo prints detailed information about the container
// for debugging purposes - useful when tests fail
func printContainerDebugInfo() {
	containerName := getContainerName()

	fmt.Println("\n==== CONTAINER DEBUG INFO ====")

	// 1. List all running containers
	fmt.Println("\n[ALL RUNNING CONTAINERS]")
	out, err := runDockerCommand("ps", "-a")
	if err != nil {
		fmt.Printf("Failed to list containers: %v\n", err)
	} else {
		fmt.Println(out)
	}

	// 2. Check our container's status
	fmt.Printf("\n[CONTAINER STATUS: %s]\n", containerName)
	out, err = runDockerCommand("inspect", "--format", "{{.State.Status}}", containerName)
	if err != nil {
		fmt.Printf("Failed to get container status: %v\n", err)
	} else {
		fmt.Printf("Status: %s\n", strings.TrimSpace(out))
	}

	// 3. Get container IP info
	fmt.Println("\n[CONTAINER NETWORK INFO]")
	out, err = runDockerCommand("inspect", "--format", "{{json .NetworkSettings}}", containerName)
	if err != nil {
		fmt.Printf("Failed to get network info: %v\n", err)
	} else {
		fmt.Printf("Network Settings: %s\n", out)
	}

	// 4. Check container logs for errors
	fmt.Println("\n[CONTAINER LOGS (last 30 lines)]")
	out, err = runDockerCommand("logs", "--tail", "30", containerName)
	if err != nil {
		fmt.Printf("Failed to get container logs: %v\n", err)
	} else {
		fmt.Println(out)
	}

	// 5. Print port mappings
	fmt.Println("\n[PORT MAPPINGS]")
	out, err = runDockerCommand("port", containerName)
	if err != nil {
		fmt.Printf("Failed to get port mappings: %v\n", err)
	} else {
		fmt.Println(out)
	}

	fmt.Println("\n==== END DEBUG INFO ====")
}
