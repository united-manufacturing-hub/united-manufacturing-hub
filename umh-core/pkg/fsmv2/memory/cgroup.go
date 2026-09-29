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

package fsmv2memory

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

type CgroupMemory struct {
	LimitBytes   int64
	CurrentBytes int64
	Unlimited    bool
}

func ReadCgroupMemory(ctx context.Context, fs filesystem.Service, cgroupBase string) (CgroupMemory, error) {
	memoryMaxData, err := fs.ReadFile(ctx, cgroupBase+"/memory.max")
	if err != nil {
		return CgroupMemory{}, fmt.Errorf("failed to read memory.max: %w", err)
	}

	limitBytes, unlimited, err := parseMemoryMax(memoryMaxData)
	if err != nil {
		return CgroupMemory{}, err
	}

	memoryCurrentData, err := fs.ReadFile(ctx, cgroupBase+"/memory.current")
	if err != nil {
		return CgroupMemory{}, fmt.Errorf("failed to read memory.current: %w", err)
	}

	currentBytes, err := parseMemoryCurrent(memoryCurrentData)
	if err != nil {
		return CgroupMemory{}, err
	}

	return CgroupMemory{LimitBytes: limitBytes, CurrentBytes: currentBytes, Unlimited: unlimited}, nil
}

func parseMemoryMax(data []byte) (limitBytes int64, unlimited bool, err error) {
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

func parseMemoryCurrent(data []byte) (currentBytes int64, err error) {
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
