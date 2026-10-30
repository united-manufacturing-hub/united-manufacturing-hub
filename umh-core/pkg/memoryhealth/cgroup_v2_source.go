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
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

const (
	v2MemoryMaxFile     = "memory.max"
	v2MemoryCurrentFile = "memory.current"
	v2NoLimitText       = "max"
)

type cgroupV2Source struct {
	fileSystem filesystem.Service
	cgroupBase string
}

func newCgroupV2Source(fileSystem filesystem.Service, cgroupBase string) *cgroupV2Source {
	return &cgroupV2Source{fileSystem: fileSystem, cgroupBase: cgroupBase}
}

// ReadCgroupMemory reads memory.max and memory.current under cgroupBase. A
// failed read returns a *CgroupReadError.
func ReadCgroupMemory(ctx context.Context, fileSystem filesystem.Service, cgroupBase string) (CgroupMemory, error) {
	return newCgroupV2Source(fileSystem, cgroupBase).read(ctx)
}

func (s *cgroupV2Source) read(ctx context.Context) (CgroupMemory, error) {
	memoryMaxData, err := readCgroupFile(ctx, s.fileSystem, s.cgroupBase, v2MemoryMaxFile)
	if err != nil {
		return CgroupMemory{}, err
	}

	memoryMaxText := strings.TrimSpace(string(memoryMaxData))
	unlimited := memoryMaxText == v2NoLimitText

	var limitBytes int64
	if !unlimited {
		limitBytes, err = parseByteCount(v2MemoryMaxFile, memoryMaxText)
		if err != nil {
			return CgroupMemory{}, &CgroupReadError{File: v2MemoryMaxFile, Outcome: ReadUnparsable, Err: err}
		}
	}

	currentBytes, err := readByteCountFile(ctx, s.fileSystem, s.cgroupBase, v2MemoryCurrentFile)
	if err != nil {
		return CgroupMemory{}, err
	}

	return CgroupMemory{LimitBytes: limitBytes, CurrentBytes: currentBytes, Unlimited: unlimited}, nil
}
