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
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// MemoryCgroupInfo contains cgroup v2 memory metrics.
type MemoryCgroupInfo struct {
	LimitBytes   int64 // Memory limit in bytes from memory.max
	CurrentBytes int64 // Current memory usage in bytes from memory.current
	Unlimited    bool  // True if memory.max is "max" (no limit set)
}

// ParseMemoryMax parses the memory.max file content.
// Returns the limit in bytes, whether the limit is unlimited ("max"), and any error.
func ParseMemoryMax(data []byte) (limitBytes int64, unlimited bool, err error) {
	s := strings.TrimSpace(string(data))
	if s == "" {
		return 0, false, errors.New("empty memory.max data")
	}

	if s == "max" {
		return 0, true, nil
	}

	limitBytes, err = strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("failed to parse memory.max value %q: %w", s, err)
	}
	if limitBytes < 0 {
		return 0, false, fmt.Errorf("negative memory.max value %q", s)
	}

	return limitBytes, false, nil
}

// ParseMemoryCurrent parses the memory.current file content.
// Returns the current memory usage in bytes.
func ParseMemoryCurrent(data []byte) (currentBytes int64, err error) {
	s := strings.TrimSpace(string(data))
	if s == "" {
		return 0, errors.New("empty memory.current data")
	}

	currentBytes, err = strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse memory.current value %q: %w", s, err)
	}
	if currentBytes < 0 {
		return 0, fmt.Errorf("negative memory.current value %q", s)
	}

	return currentBytes, nil
}

// getCgroupMemoryInfo reads cgroup v2 memory limits and current usage.
func (c *ContainerMonitorService) getCgroupMemoryInfo(ctx context.Context) (*MemoryCgroupInfo, error) {
	info := &MemoryCgroupInfo{}

	memMaxData, err := os.ReadFile("/sys/fs/cgroup/memory.max")
	if err != nil {
		return nil, fmt.Errorf("failed to read memory.max: %w", err)
	}

	limitBytes, unlimited, err := ParseMemoryMax(memMaxData)
	if err != nil {
		return nil, err
	}

	info.LimitBytes = limitBytes
	info.Unlimited = unlimited

	memCurrentData, err := os.ReadFile("/sys/fs/cgroup/memory.current")
	if err != nil {
		return nil, fmt.Errorf("failed to read memory.current: %w", err)
	}

	currentBytes, err := ParseMemoryCurrent(memCurrentData)
	if err != nil {
		return nil, err
	}

	info.CurrentBytes = currentBytes

	return info, nil
}
