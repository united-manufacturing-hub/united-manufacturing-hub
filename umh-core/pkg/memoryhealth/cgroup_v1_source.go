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

package memoryhealth

import (
	"context"
	"math"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

const (
	v1MemoryLimitFile = "memory/memory.limit_in_bytes"
	v1MemoryUsageFile = "memory/memory.usage_in_bytes"

	largestKernelPageBytes = 64 << 10
	v1NoLimitFloorBytes    = math.MaxInt64 - largestKernelPageBytes
)

type cgroupV1Source struct {
	fileSystem filesystem.Service
	cgroupBase string
}

func newCgroupV1Source(fileSystem filesystem.Service, cgroupBase string) *cgroupV1Source {
	return &cgroupV1Source{fileSystem: fileSystem, cgroupBase: cgroupBase}
}

func (s *cgroupV1Source) read(ctx context.Context) (CgroupMemory, error) {
	limitBytes, err := readByteCountFile(ctx, s.fileSystem, s.cgroupBase, v1MemoryLimitFile)
	if err != nil {
		return CgroupMemory{}, err
	}

	usageBytes, err := readByteCountFile(ctx, s.fileSystem, s.cgroupBase, v1MemoryUsageFile)
	if err != nil {
		return CgroupMemory{}, err
	}

	if limitBytes >= v1NoLimitFloorBytes {
		return CgroupMemory{CurrentBytes: usageBytes, Unlimited: true}, nil
	}

	return CgroupMemory{LimitBytes: limitBytes, CurrentBytes: usageBytes}, nil
}

func (s *cgroupV1Source) readPressure(context.Context) diagnosis.Reading {
	return diagnosis.Unknown()
}
