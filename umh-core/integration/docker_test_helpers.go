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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/gomega"
)

const (
	containerName = "umh-core"
	imageName     = "umh-core:latest"
	metricsURL    = "http://localhost:8081/metrics"
)

// BuildAndRunContainer rebuilds your Docker image, starts the container, etc.
func BuildAndRunContainer(configFilePath string, memory string) error {
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
		"-v", fmt.Sprintf("%s/data:/data", GetCurrentDir()),
		"-e", "LOGGING_LEVEL=debug",
		"-p", "8081:8080",
		"-p", "8082:8082",
		imageName,
	)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Could not run container: %s", out))

	// 4. Wait for the container to be healthy
	return waitForMetrics()
}

// printContainerLogs prints the logs from the container
func printContainerLogs() {
	out, err := runDockerCommand("logs", containerName)
	if err != nil {
		fmt.Printf("Failed to get container logs: %v\n", err)
		return
	}
	fmt.Printf("Container logs:\n%s\n", out)
}

// StopContainer stops and removes your container
func StopContainer() {
	runDockerCommand("stop", containerName)
	// don't delete it, we want to keep the logs
}

func runDockerCommand(args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
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
	dataDir := filepath.Join(GetCurrentDir(), "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("failed to create data dir: %w", err)
	}
	configPath := filepath.Join(dataDir, "config.yaml")
	return os.WriteFile(configPath, []byte(yamlContent), 0o644)
}
