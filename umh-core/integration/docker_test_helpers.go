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

	// Test-specific config file path
	configFilePath string
	configOnce     sync.Once
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

// getConfigFilePath returns a unique config file path for this test run
func getConfigFilePath() string {
	configOnce.Do(func() {
		// Use the same suffix as the container name for consistency
		suffix := strings.TrimPrefix(getContainerName(), containerBaseName+"-")

		// Create a umh-config directory within the current directory
		currentDir := GetCurrentDir()
		tmpDir := filepath.Join(currentDir, "umh-config")

		// Create the directory
		err := os.MkdirAll(tmpDir, 0o777)
		if err != nil {
			// Fallback to current directory if directory creation fails
			tmpDir = currentDir
		}

		configFilePath = filepath.Join(tmpDir, fmt.Sprintf("config-%s.yaml", suffix))
	})
	return configFilePath
}

// GetMetricsURL returns the URL for the metrics endpoint, using the container's IP if possible
func GetMetricsURL() string {
	portMu.Lock()
	port := metricsPort
	portMu.Unlock()
	if port == 0 {
		port = 8080 // Fallback to default port
	}
	fmt.Printf("Using localhost URL with host port: http://%s:%d/metrics\n", "localhost", port)
	return fmt.Sprintf("http://%s:%d/metrics", "localhost", port)
}

// GetGoldenServiceURL returns the URL for the golden service
func GetGoldenServiceURL() string {
	portMu.Lock()
	port := goldenPort
	portMu.Unlock()
	if port == 0 {
		port = 8082 // Fallback to default port
	}
	fmt.Printf("Using localhost URL with host port: http://%s:%d/health\n", "localhost", port)
	return fmt.Sprintf("http://%s:%d", "localhost", port)
}

// writeConfigFile writes the given YAML content to a config file for the container to read.
// If containerName is not empty, it will also write the config directly into that container.
func writeConfigFile(yamlContent string, containerName ...string) error {
	configPath := getConfigFilePath()

	// Always remove any existing file first
	os.Remove(configPath)

	// Ensure the directory exists with wide permissions
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	// Write the file with permissions that allow anyone to read/write
	if err := os.WriteFile(configPath, []byte(yamlContent), 0o666); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// If container name is provided, also write directly to container
	if len(containerName) > 0 && containerName[0] != "" {
		container := containerName[0]
		fmt.Printf("Writing config directly to container %s...\n", container)

		// Escape double quotes and newlines for echo command
		escapedConfig := strings.ReplaceAll(yamlContent, "\"", "\\\"")
		escapedConfig = strings.ReplaceAll(escapedConfig, "$", "\\$")

		// Create the config file in the container
		cmd := fmt.Sprintf("bash -c \"echo '%s' > /data/config.yaml\"", escapedConfig)
		out, err := runDockerCommand("exec", container, "bash", "-c", cmd)
		if err != nil {
			fmt.Printf("Failed to write config to container: %v\n%s\n", err, out)
			return fmt.Errorf("failed to write config to container: %w", err)
		}

		// Verify the config was written correctly
		out, err = runDockerCommand("exec", container, "cat", "/data/config.yaml")
		if err != nil {
			fmt.Printf("Failed to verify config in container: %v\n", err)
			return fmt.Errorf("failed to verify config in container: %w", err)
		}
	}

	return nil
}

// BuildAndRunContainer rebuilds your Docker image, starts the container, etc.
func BuildAndRunContainer(configYaml string, memory string) error {
	// Get the unique container name for this test run
	containerName := getContainerName()

	fmt.Printf("\n=== STARTING CONTAINER BUILD AND RUN ===\n")
	fmt.Printf("Container name: %s\n", containerName)
	fmt.Printf("Memory limit: %s\n", memory)

	// Generate random ports to avoid conflicts
	// Use PID to create somewhat unique ports, but with a limited range to avoid system port limits
	metricsPrt := 8081 + (os.Getpid() % 1000) // Base port 8081 + offset based on PID
	goldenPrt := 8082 + (os.Getpid() % 1000)  // Base port 8082 + offset based on PID

	fmt.Printf("Port mappings - Host:Container\n")
	fmt.Printf("- Metrics: %d:8080\n", metricsPrt)
	fmt.Printf("- Golden: %d:8082\n", goldenPrt)

	// Store these ports for later use
	portMu.Lock()
	metricsPort = metricsPrt
	goldenPort = goldenPrt
	portMu.Unlock()

	// 1. Stop/Remove the old container if any
	fmt.Println("Cleaning up any previous containers with the same name...")
	out, err := runDockerCommand("rm", "-f", containerName) // ignoring error
	if err != nil {
		fmt.Printf("Note: Container removal returned: %v, output: %s (this may be normal)\n", err, out)
	}

	out, err = runDockerCommand("stop", containerName) // ignoring error
	if err != nil {
		fmt.Printf("Note: Container stop returned: %v, output: %s (this may be normal)\n", err, out)
	}

	// 2. Build image
	fmt.Println("Building Docker image...")
	coreDir := filepath.Dir(GetCurrentDir())
	dockerfilePath := filepath.Join(coreDir, "Dockerfile")

	fmt.Printf("Core directory: %s\n", coreDir)
	fmt.Printf("Dockerfile path: %s\n", dockerfilePath)

	out, err = runDockerCommand("build", "-t", imageName, "-f", dockerfilePath, coreDir)
	if err != nil {
		fmt.Printf("Docker build failed: %v\n", err)
		fmt.Printf("Build output:\n%s\n", out)
		return fmt.Errorf("Docker build failed: %s", out)
	}
	fmt.Println("Docker build successful")

	// 3. First just write the config to the local file
	fmt.Println("Writing local config file...")
	if err := writeConfigFile(configYaml); err != nil {
		return fmt.Errorf("failed to write local config file: %w", err)
	}

	// 4. Run container WITHOUT mounting the config file
	fmt.Println("Starting container...")
	out, err = runDockerCommand(
		"run", "-d",
		"--name", containerName,
		"-e", "LOGGING_LEVEL=debug",
		// Map the host ports to the container's fixed ports
		"-p", fmt.Sprintf("%d:8080", metricsPrt), // Map host's dynamic port to container's fixed metrics port
		"-p", fmt.Sprintf("%d:8082", goldenPrt), // Map host's dynamic port to container's golden service port
		imageName,
	)
	if err != nil {
		fmt.Printf("Container start failed: %v\n", err)
		fmt.Printf("Container run output:\n%s\n", out)
		return fmt.Errorf("could not run container: %s", out)
	}
	fmt.Printf("Container started with ID: %s\n", strings.TrimSpace(out))

	// 5. Now write the config directly to the container
	if err := writeConfigFile(configYaml, containerName); err != nil {
		return fmt.Errorf("failed to write config to container: %w", err)
	}

	// 6. Verify the container is actually running
	out, err = runDockerCommand("inspect", "--format", "{{.State.Status}}", containerName)
	if err != nil {
		fmt.Printf("Failed to inspect container: %v\n", err)
	} else {
		containerState := strings.TrimSpace(out)
		fmt.Printf("Container state: %s\n", containerState)
		if containerState != "running" {
			fmt.Println("WARNING: Container is not in running state!")
			printContainerDebugInfo()
			return fmt.Errorf("container is in %s state, not running", containerState)
		}
	}

	// 7. Wait for the container to be healthy
	fmt.Println("Waiting for metrics endpoint to become available...")
	return waitForMetrics()
}

// cleanupConfigFile removes the test-specific config file
func cleanupConfigFile() {
	if configFilePath != "" {
		// Just attempt to remove it, ignoring errors
		os.Remove(configFilePath)

		// Try to clean up the umh-config directory if it exists
		dir := filepath.Dir(configFilePath)
		if strings.HasSuffix(dir, "umh-config") {
			// This will only succeed if the directory is empty
			os.Remove(dir)
		}
	}
}

// getContainerIP returns the IP address of a running container
func getContainerIP(container string) (string, error) {
	// First try the traditional IPAddress field
	out, err := runDockerCommand("inspect", "--format", "{{.NetworkSettings.IPAddress}}", container)
	if err != nil {
		return "", fmt.Errorf("failed to get container IP: %w", err)
	}

	// Trim whitespace
	ip := strings.TrimSpace(out)

	// If we got an empty IP, try to get it from the networks
	if ip == "" {
		// Try to get from bridge network (common default)
		out, err = runDockerCommand("inspect", "--format", "{{.NetworkSettings.Networks.bridge.IPAddress}}", container)
		if err == nil {
			ip = strings.TrimSpace(out)
		}
	}

	// If still empty, try "networks" plural for Docker Compose setups
	if ip == "" {
		out, err = runDockerCommand("inspect", "--format", "{{range $net, $conf := .NetworkSettings.Networks}}{{$conf.IPAddress}} {{end}}", container)
		if err == nil {
			// This might return multiple IPs, take the first non-empty one
			for _, possibleIP := range strings.Fields(out) {
				if possibleIP != "" {
					ip = possibleIP
					break
				}
			}
		}
	}

	// If we still don't have an IP, log a warning
	if ip == "" {
		fmt.Printf("WARNING: Could not determine container IP for %s. Will use localhost instead.\n", container)
	} else {
		fmt.Printf("Found container IP for %s: %s\n", container, ip)
	}

	return ip, nil
}

// printContainerLogs prints the logs from the container
func printContainerLogs() {
	containerName := getContainerName()
	fmt.Printf("Test failed, printing container logs:\n")

	// 1. Regular container logs (stdout/stderr)
	fmt.Printf("\n=== DOCKER CONTAINER LOGS ===\n")
	out, err := runDockerCommand("logs", containerName)
	if err != nil {
		fmt.Printf("Failed to get container logs: %v\n", err)
	} else {
		fmt.Printf("%s\n", out)
	}

	// 2. Internal UMH Core logs
	fmt.Printf("\n=== UMH CORE INTERNAL LOGS ===\n")
	out, err = runDockerCommand("exec", containerName, "cat", "/data/logs/umh-core/current")
	if err != nil {
		fmt.Printf("Failed to get UMH Core internal logs: %v\n", err)
	} else if out == "" {
		fmt.Printf("UMH Core internal logs are empty or not found\n")
	} else {
		fmt.Printf("%s\n", out)
	}

	// 3. Golden Service logs
	fmt.Printf("\n=== GOLDEN SERVICE INTERNAL LOGS ===\n")
	out, err = runDockerCommand("exec", containerName, "cat", "/data/logs/golden-service/current")
	if err != nil {
		fmt.Printf("Failed to get Golden Service internal logs: %v\n", err)
	} else if out == "" {
		fmt.Printf("Golden Service internal logs are empty or not found\n")
	} else {
		fmt.Printf("%s\n", out)
	}

	// 4. List all available log files for reference
	fmt.Printf("\n=== AVAILABLE LOG FILES ===\n")
	out, err = runDockerCommand("exec", containerName, "find", "/data/logs", "-type", "f")
	if err != nil {
		fmt.Printf("Failed to list log files: %v\n", err)
	} else if out == "" {
		fmt.Printf("No log files found in /data/logs\n")
	} else {
		fmt.Printf("%s\n", out)
	}
}

// StopContainer stops and removes your container
func StopContainer() {
	containerName := getContainerName()
	// First stop the container
	runDockerCommand("stop", containerName)
	// Then remove it
	runDockerCommand("rm", "-f", containerName)

	// Clean up config file
	cleanupConfigFile()

}

func CleanupDockerBuildCache() {
	// If running in CI environment, also clean up Docker build cache
	if os.Getenv("CI") == "true" {
		fmt.Println("Running in CI environment, cleaning up Docker build cache...")
		runDockerCommand("builder", "prune", "-f")
		// Only prune non-running containers/images to avoid conflicts with other tests
		runDockerCommand("system", "prune", "-f")
	}
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

func runDockerCommand(args ...string) (string, error) {
	fmt.Printf("Running docker command: %v\n", args)
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
