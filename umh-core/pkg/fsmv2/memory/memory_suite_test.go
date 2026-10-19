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
	"io/fs"
	"strconv"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/memoryhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

func TestMemory(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "fsmv2memory Suite")
}

const (
	decimalLimitBytes int64 = 1000000000

	fixtureCgroupBase = "/sys/fs/cgroup"
)

var errHostUnreadable = errors.New("host memory unreadable")

func percentOfDecimalLimitText(percent int64) string {
	return strconv.FormatInt(decimalLimitBytes*percent/100, 10) + "\n"
}

func cgroupAtPercent(usedPercent int64) map[string]string {
	return map[string]string{
		fixtureCgroupBase + "/memory.max":     strconv.FormatInt(decimalLimitBytes, 10) + "\n",
		fixtureCgroupBase + "/memory.current": percentOfDecimalLimitText(usedPercent),
	}
}

func fixtureFilesystem(files map[string]string) *filesystem.MockFileSystem {
	return filesystem.NewMockFileSystem().WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		content, ok := files[path]
		if !ok {
			return nil, fs.ErrNotExist
		}

		return []byte(content), nil
	})
}

func unreadableHostMemory(context.Context) (uint64, uint64, error) {
	return 0, 0, errHostUnreadable
}

func newTestDeps(fileSystem filesystem.Service, hostMemory memoryhealth.HostMemoryReader) *MemoryDeps {
	identity := deps.Identity{ID: "memory-test", WorkerType: WorkerType}
	baseDependencies := deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, identity)

	return newMemoryDeps(baseDependencies, memoryhealth.NewSampler(fileSystem, fixtureCgroupBase, hostMemory))
}
