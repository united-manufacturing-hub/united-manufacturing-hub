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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

type cgroupVersion int

const (
	cgroupVersionUnresolved cgroupVersion = iota
	cgroupV2
	cgroupV1
)

func detectCgroupVersion(ctx context.Context, fileSystem filesystem.Service, cgroupBase string) cgroupVersion {
	if fileExists(ctx, fileSystem, cgroupBase+"/"+v2MemoryMaxFile) {
		return cgroupV2
	}

	if fileExists(ctx, fileSystem, cgroupBase+"/"+v1MemoryLimitFile) {
		return cgroupV1
	}

	return cgroupVersionUnresolved
}

func fileExists(ctx context.Context, fileSystem filesystem.Service, path string) bool {
	exists, err := fileSystem.FileExists(ctx, path)

	return err == nil && exists
}
