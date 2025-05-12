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

//go:build !darwin
// +build !darwin

package container_monitor

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// getMacOSAdjustedDiskMetrics retrieves adjusted disk metrics using unix.Statfs.
// It uses stat.Frsize when available, since on Docker Desktop for macOS the reported
// Bsize is often 1024× larger than the actual block size.
// We use unix.Statfs directly instead of relying on gopsutil's disk.Usage because:
// 1. It gives us direct access to the Frsize field which is crucial for proper block size calculation
// 2. gopsutil doesn't handle the Docker Desktop for macOS edge case correctly
func (c *ContainerMonitorService) getMacOSAdjustedDiskMetrics() (usedBytes, totalBytes uint64, err error) {
	var stat unix.Statfs_t
	err = unix.Statfs(c.dataPath, &stat)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to stat filesystem at %s: %w", c.dataPath, err)
	}

	// Use Frsize if available; it represents the fundamental block size for macOS.
	bSize := uint64(stat.Bsize)
	if stat.Frsize > 0 {
		bSize = uint64(stat.Frsize)
	}

	// Compute total and used bytes based on the corrected block size.
	totalBytes = stat.Blocks * bSize
	usedBytes = (stat.Blocks - stat.Bfree) * bSize

	return usedBytes, totalBytes, nil
}
