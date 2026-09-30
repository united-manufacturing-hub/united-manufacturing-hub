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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

func TestMemory(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "fsmv2memory Suite")
}

const (
	halfGiBBytes  int64 = 536870912
	oneGiBBytes   int64 = 1073741824
	twoGiBBytes   int64 = 2147483648
	threeGiBBytes int64 = 3221225472
	eightGiBBytes int64 = 8589934592

	decimalLimitBytes int64 = 1000000000

	fixtureCgroupBase = "/sys/fs/cgroup"
)

var errHostUnreadable = errors.New("host memory unreadable")

func bytesText(bytes int64) string {
	return strconv.FormatInt(bytes, 10) + "\n"
}

func percentOfDecimalLimitText(percent int64) string {
	return bytesText(decimalLimitBytes * percent / 100)
}

func cgroupFiles(memoryMax, memoryCurrent string) map[string]string {
	return map[string]string{
		fixtureCgroupBase + "/memory.max":     memoryMax,
		fixtureCgroupBase + "/memory.current": memoryCurrent,
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

func hostMemoryOf(usedBytes, totalBytes int64) HostMemoryReader {
	return func(context.Context) (uint64, uint64, error) {
		return uint64(usedBytes), uint64(totalBytes), nil
	}
}

func unreadableHostMemory(context.Context) (uint64, uint64, error) {
	return 0, 0, errHostUnreadable
}

func newTestDeps(fileSystem filesystem.Service, hostMemory HostMemoryReader) *MemoryDeps {
	identity := deps.Identity{ID: "memory-test", WorkerType: WorkerType}

	return &MemoryDeps{
		BaseDependencies: deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, identity),
		fileSystem:       fileSystem,
		hostMemory:       hostMemory,
	}
}
