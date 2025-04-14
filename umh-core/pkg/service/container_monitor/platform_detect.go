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

package container_monitor

import (
	"os"
	"strings"
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
)

var (
	isDockerDesktopMacOnce sync.Once
	isDockerDesktopMacVal  bool
)

// IsDockerDesktopMac determines whether the current environment is likely
// Docker Desktop running on macOS by inspecting /proc/version for the signature "linuxkit".
// Docker Desktop on macOS runs containers inside a Linux VM built with LinuxKit.
// The result is cached after the first call to avoid repeated reads of /proc/version.
func IsDockerDesktopMac() bool {
	isDockerDesktopMacOnce.Do(func() {
		// Read the contents of /proc/version.
		data, err := os.ReadFile("/proc/version")
		if err != nil {
			// If we cannot read /proc/version, fall back to false.
			isDockerDesktopMacVal = false
			return
		}
		// Check for "linuxkit" which is indicative of Docker Desktop on macOS.
		isDockerDesktopMacVal = strings.Contains(string(data), "linuxkit")
		logger := logger.For("platform_detect")
		if isDockerDesktopMacVal {
			logger.Infof("Detected Docker Desktop on macOS! Using macOS-adjusted disk metrics.")
		}
	})
	return isDockerDesktopMacVal
}
