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
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint: staticcheck // Ginkgo is designed to be used with dot imports
)

const (
	containerBaseName = "umh-core"
)

var (
	// containerName is a unique name for this test run.
	containerName string
	containerOnce sync.Once

	// Store port mappings for each container.
	metricsPort uint16
	goldenPort  uint16
	portMu      sync.Mutex

	// Test-specific config file path.
	configFilePath string
	configOnce     sync.Once
)

var imageNameOnce sync.Once
var imageName string

func getImageName() string {
	imageNameOnce.Do(func() {
		imageName = "umh-core:latest-" + time.Now().Format("20060102150405")
		fmt.Printf("Using image name: %s\n", imageName)
	})

	return imageName
}

// getContainerName returns a unique container name for this test run.
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

// getConfigFilePath returns a unique config file path for this test run.
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

// GetMetricsURL returns the URL for the metrics endpoint, using the container's IP if possible.
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

// GetGoldenServiceURL returns the URL for the golden service.
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
	err := os.Remove(configPath)
	if err != nil {
		if !os.IsNotExist(err) { // Ignore if the file does not exist
			return fmt.Errorf("failed to remove existing config file: %w", err)
		}
	}

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

		// Create a temporary file with the actual config content
		tmpFile := configPath + ".tmp"
		if err := os.WriteFile(tmpFile, []byte(yamlContent), 0o666); err != nil {
			return fmt.Errorf("failed to write temp config file: %w", err)
		}

		defer func() {
			err := os.Remove(tmpFile)
			if err != nil {
				GinkgoWriter.Printf("Failed to remove temp config file: %v\n", err)
			}
		}()

		// Copy the file directly to the container
		out, err := runDockerCommand("cp", tmpFile, container+":/data/config.yaml")
		if err != nil {
			GinkgoWriter.Printf("Failed to copy config to container: %v\n%s\n", err, out)

			return fmt.Errorf("failed to copy config to container: %w", err)
		}
	}

	return nil
}

func buildContainer() error {
	GinkgoWriter.Println("Building Docker image...")
	// Check if this image already exists
	out, err := runDockerCommand("images", "-q", getImageName())

	var exists bool

	if err != nil {
		GinkgoWriter.Printf("Failed to check if image exists: %v\n", err)
		// If we fail for any reason, assume the image does not exist
		exists = false
	}

	if out != "" {
		exists = true
	}

	if exists {
		GinkgoWriter.Printf("Image %s already exists, skipping build\n", getImageName())

		return nil
	}

	coreDir := filepath.Dir(GetCurrentDir())
	dockerfilePath := filepath.Join(coreDir, "Dockerfile")

	GinkgoWriter.Printf("Core directory: %s\n", coreDir)
	GinkgoWriter.Printf("Dockerfile path: %s\n", dockerfilePath)

	// Let's just use make build with our own tag
	fullImageName := getImageName()
	// Split in name and tag
	imageNameParts := strings.Split(fullImageName, ":")
	imageName := imageNameParts[0]
	tag := imageNameParts[1]

	var outmake []byte

	cmd := exec.Command("make", "build", "IMAGE_NAME="+imageName, "TAG="+tag)
	cmd.Dir = coreDir // Set working directory to coreDir

	outmake, err = cmd.Output()
	if err != nil {
		fmt.Printf("Docker build failed: %v\n", err)
		fmt.Printf("Build output:\n%s\n", outmake)

		return fmt.Errorf("docker build failed: %w", err)
	}

	GinkgoWriter.Printf("Output of make build: %s\n", outmake)

	GinkgoWriter.Println("Docker build successful")

	return nil
}

// BuildAndRunContainer rebuilds your Docker image, starts the container, etc.
func BuildAndRunContainer(configYaml string, memory string, cpus uint) error {
	// Get the unique container name for this test run
	containerName := getContainerName()

	if memory == "" {
		// Fail if memory is not set
		return errors.New("memory limit is not set")
	}

	if cpus == 0 {
		// Fail if cpus are not set
		return errors.New("cpu limit is not set")
	}

	GinkgoWriter.Printf("\n=== STARTING CONTAINER BUILD AND RUN ===\n")
	GinkgoWriter.Printf("Container name: %s\n", containerName)
	GinkgoWriter.Printf("Memory limit: %s\n", memory)
	GinkgoWriter.Printf("CPU limit: %d\n", cpus)

	// Generate random ports to avoid conflicts
	// Use PID to create somewhat unique ports, but with a limited range to avoid system port limits
	metricsPrt := 8081 + (os.Getpid() % 1000) // Base port 8081 + offset based on PID
	goldenPrt := 8082 + (os.Getpid() % 1000)  // Base port 8082 + offset based on PID

	GinkgoWriter.Printf("Port mappings - Host:Container\n")
	GinkgoWriter.Printf("- Metrics: %d:8080\n", metricsPrt)
	GinkgoWriter.Printf("- Golden: %d:8082\n", goldenPrt)

	// Store these ports for later use
	portMu.Lock()

	metricsPort = uint16(metricsPrt)
	goldenPort = uint16(goldenPrt)

	portMu.Unlock()

	// 1. Stop/Remove the old container if any
	GinkgoWriter.Println("Cleaning up any previous containers with the same name...")

	out, err := runDockerCommand("rm", "-f", containerName) // ignoring error
	if err != nil {
		GinkgoWriter.Printf("Note: Container removal returned: %v, output: %s (this may be normal)\n", err, out)
	}

	out, err = runDockerCommand("stop", containerName) // ignoring error
	if err != nil {
		GinkgoWriter.Printf("Note: Container stop returned: %v, output: %s (this may be normal)\n", err, out)
	}

	// 2. Build image
	if err := buildContainer(); err != nil {
		return err
	}

	// 3. Run container WITHOUT mounting the config file (but we do need to mount the /data/redpanda folder, otherwise it cannot start)
	tmpRedpandaDir := filepath.Join(getTmpDir(), containerName, "redpanda")
	tmpLogsDir := filepath.Join(getTmpDir(), containerName, "logs")

	// 4. Create the directories
	if err := os.MkdirAll(tmpRedpandaDir, 0o777); err != nil {
		return fmt.Errorf("failed to create redpanda dir: %w", err)
	}

	if err := os.MkdirAll(tmpLogsDir, 0o777); err != nil {
		return fmt.Errorf("failed to create logs dir: %w", err)
	}

	// 5. Create the container WITHOUT starting it
	GinkgoWriter.Println("Creating container (without starting it)...")

	// ----------------------------------------
	//  Configure CFS bandwidth for the test
	// ----------------------------------------

	// Length of one CFS "accounting window" (cpu.cfs_period_us).
	// 20 000 µs = 20 ms  →  the longest time the container can be throttled
	// if it burns its entire quota in one burst.
	cpuPeriod := uint(20_000) // 20 ms

	// Total CPU-time the container may consume inside **one** period
	// (cpu.cfs_quota_us).  quota = cpus × period, so if 'cpus' is 2 you
	// grant 40 ms every 20 ms → an average of 2 fully-loaded logical cores.
	cpuQuota := cpus * cpuPeriod

	out, err = runDockerCommand(
		"create",
		"--name", containerName,
		"-e", "LOGGING_LEVEL=debug",
		// Map the host ports to the container's fixed ports
		"-p", fmt.Sprintf("%d:8080", metricsPrt), // Map host's dynamic port to container's fixed metrics port
		"-p", fmt.Sprintf("%d:8082", goldenPrt), // Map host's dynamic port to container's golden service port
		"--memory", memory,
		"--cpu-period", strconv.FormatUint(uint64(cpuPeriod), 10), // 20 ms CFS window
		"--cpu-quota", strconv.FormatUint(uint64(cpuQuota), 10), // e.g. 40 ms budget ⇒ 2 vCPUs
		"-v", tmpRedpandaDir+":/data/redpanda",
		"-v", tmpLogsDir+":/logs",
		getImageName(),
	)
	if err != nil {
		GinkgoWriter.Printf("Container creation failed: %v\n", err)
		GinkgoWriter.Printf("Container create output:\n%s\n", out)

		return fmt.Errorf("could not create container: %s", out)
	}

	GinkgoWriter.Printf("Container created with ID: %s\n", strings.TrimSpace(out))

	// 6. Copy the config file to the container BEFORE starting it
	GinkgoWriter.Println("Copying config file to container...")

	err = writeConfigFile(configYaml, containerName)
	if err != nil {
		return fmt.Errorf("failed to write config to container: %w", err)
	}

	// 7. Start the container
	GinkgoWriter.Println("Starting the container...")

	out, err = runDockerCommand("start", containerName)
	if err != nil {
		GinkgoWriter.Printf("Container start failed: %v\n", err)
		GinkgoWriter.Printf("Container start output:\n%s\n", out)

		return fmt.Errorf("could not start container: %s", out)
	}

	GinkgoWriter.Println("Container started successfully")

	// 8. Verify the container is actually running
	out, err = runDockerCommand("inspect", "--format", "{{.State.Status}}", containerName)
	if err != nil {
		GinkgoWriter.Printf("Failed to inspect container: %v\n", err)
	} else {
		containerState := strings.TrimSpace(out)
		GinkgoWriter.Printf("Container state: %s\n", containerState)

		if containerState != "running" {
			GinkgoWriter.Println("WARNING: Container is not in running state!")
			printContainerDebugInfo()

			return fmt.Errorf("container is in %s state, not running", containerState)
		}
	}

	// 9. Wait for the container to be healthy
	GinkgoWriter.Println("Waiting for metrics endpoint to become available...")

	return waitForMetrics()
}

// cleanupConfigFile removes the test-specific config file.
func cleanupConfigFile() {
	if configFilePath != "" {
		// Just attempt to remove it, ignoring errors
		err := os.Remove(configFilePath)
		if err != nil {
			GinkgoWriter.Printf("Failed to remove config file: %v\n", err)
		}

		// Try to clean up the umh-config directory if it exists
		dir := filepath.Dir(configFilePath)
		if strings.HasSuffix(dir, "umh-config") {
			// This will only succeed if the directory is empty
			err := os.Remove(dir)
			if err != nil {
				GinkgoWriter.Printf("Failed to remove umh-config directory: %v\n", err)
			}
		}
	}
}

// getTmpDir returns the temporary directory for a container.
func getTmpDir() string {
	tmpDir := "/tmp"
	// If we are in a devcontainer, use the workspace as tmp dir
	if os.Getenv("REMOTE_CONTAINERS") != "" || os.Getenv("CODESPACE_NAME") != "" || os.Getenv("USER") == "vscode" {
		// Use the current working directory to determine the correct tmp path
		currentDir := GetCurrentDir()
		tmpDir = filepath.Join(currentDir, "tmp")
	}

	return tmpDir
}

// cleanupTmpDirs cleans up the temporary directories for a container.
func cleanupTmpDirs(containerName string) {
	filepath := filepath.Join(getTmpDir(), containerName)
	GinkgoWriter.Printf("Cleaning up temporary directories for container %s (%s)\n", containerName, filepath)

	err := os.RemoveAll(filepath)
	if err != nil {
		GinkgoWriter.Printf("Failed to remove temporary directories: %v\n", err)
	}
}

// printContainerLogs prints the logs from the container.
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

	out, err = runDockerCommand("exec", containerName, "cat", "/logs/umh-core/current")
	switch {
	case err != nil:
		fmt.Printf("Failed to get UMH Core internal logs: %v\n", err)
	case out == "":
		fmt.Printf("UMH Core internal logs are empty or not found\n")
	default:
		fmt.Printf("%s\n", out)
	}

	// 3. Golden Service logs
	fmt.Printf("\n=== GOLDEN SERVICE INTERNAL LOGS ===\n")

	out, err = runDockerCommand("exec", containerName, "cat", "/logs/golden-service/current")
	switch {
	case err != nil:
		fmt.Printf("Failed to get Golden Service internal logs: %v\n", err)
	case out == "":
		fmt.Printf("Golden Service internal logs are empty or not found\n")
	default:
		fmt.Printf("%s\n", out)
	}

	// 4. Golden Benthos logs
	fmt.Printf("\n=== GOLDEN BENTHOS INTERNAL LOGS ===\n")

	out, err = runDockerCommand("exec", containerName, "cat", "/logs/golden-benthos/current")
	switch {
	case err != nil:
		fmt.Printf("Failed to get Benthos internal logs: %v\n", err)
	case out == "":
		fmt.Printf("Benthos internal logs are empty or not found\n")
	default:
		fmt.Printf("%s\n", out)
	}

	// 5. List all available log files for reference
	fmt.Printf("\n=== AVAILABLE LOG FILES ===\n")

	out, err = runDockerCommand("exec", containerName, "find", "/logs", "-type", "f")
	switch {
	case err != nil:
		fmt.Printf("Failed to list log files: %v\n", err)
	case out == "":
		fmt.Printf("No log files found in /logs\n")
	default:
		fmt.Printf("%s\n", out)
	}

	// 6. Copy out ALL logs to a tmp dir (cia docker cp)
	// The path is not randomized, so we can easily find in the GitHub Actions workflow
	tmpDir := filepath.Join(getTmpDir(), "logs")

	// If the dir exists, remove it
	if _, err := os.Stat(tmpDir); err == nil {
		err = os.RemoveAll(tmpDir)
		if err != nil {
			fmt.Printf("Failed to remove tmp dir: %v\n", err)
		}
	}
	// Create the dir
	err = os.MkdirAll(tmpDir, 0o777)
	if err != nil {
		fmt.Printf("Failed to create tmp dir: %v\n", err)
	} else {
		containerName := getContainerName()

		_, err = runDockerCommand("cp", containerName+":/logs", tmpDir)
		if err != nil {
			fmt.Printf("Failed to copy out logs: %v\n", err)
		}

		fmt.Printf("Copied logs to %s\n", tmpDir)
	}
}

// PrintLogsAndStopContainer stops and removes your container.
func PrintLogsAndStopContainer() {
	containerName := getContainerName()

	if CurrentSpecReport().Failed() {
		fmt.Println("Test failed, printing container logs:")
		printContainerLogs()

		// Print the latest YAML config
		fmt.Println("\nLatest YAML config at time of failure:")

		configPath := getConfigFilePath()

		config, err := os.ReadFile(configPath)
		if err != nil {
			fmt.Printf("Failed to read config file %s: %v\n", configPath, err)
		} else {
			fmt.Println(string(config))
		}

		// Save logs for debugging
		containerNameInError := getContainerName()
		fmt.Printf("\nTest failed. Container name: %s\n", containerNameInError)
	}
	// First stop the container
	out, err := runDockerCommand("stop", containerName)
	if err != nil {
		GinkgoWriter.Printf("Failed to stop container: %v\n", err)
		GinkgoWriter.Printf("Stop container output:\n%s\n", out)
	}
	// Then remove it
	out, err = runDockerCommand("rm", "-f", containerName)
	if err != nil {
		GinkgoWriter.Printf("Failed to remove container: %v\n", err)
		GinkgoWriter.Printf("Remove container output:\n%s\n", out)
	}
	// Clean up config file
	cleanupConfigFile()
}

func CleanupDockerBuildCache() {
	// If running in CI environment, also clean up Docker build cache
	if os.Getenv("CI") == "true" {
		fmt.Println("Running in CI environment, cleaning up Docker build cache...")

		out, err := runDockerCommand("builder", "prune", "-f")
		if err != nil {
			GinkgoWriter.Printf("Failed to prune Docker builder cache: %v\n", err)
			GinkgoWriter.Printf("Prune builder cache output:\n%s\n", out)
		}
		// Only prune non-running containers/images to avoid conflicts with other tests
		out, err = runDockerCommand("system", "prune", "-f")
		if err != nil {
			GinkgoWriter.Printf("Failed to prune Docker system: %v\n", err)
			GinkgoWriter.Printf("Prune system output:\n%s\n", out)
		}
	}
}

// printContainerDebugInfo prints detailed information about the container
// for debugging purposes - useful when tests fail.
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
	return runDockerCommandWithCtx(context.Background(), args...)
}

func runDockerCommandWithCtx(ctx context.Context, args ...string) (string, error) {
	fmt.Printf("Running docker command: %v\n", args)
	// Check if we use docker or podman
	dockerCmd := "docker"
	if _, err := exec.LookPath("podman"); err == nil {
		dockerCmd = "podman"
	}

	cmd := exec.CommandContext(ctx, dockerCmd, args...)

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
