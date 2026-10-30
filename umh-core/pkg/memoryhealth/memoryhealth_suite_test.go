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

package memoryhealth_test

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"strconv"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/memoryhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

func TestMemoryHealth(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "memoryhealth Suite")
}

const (
	halfGiBBytes  int64 = 536870912
	oneGiBBytes   int64 = 1073741824
	twoGiBBytes   int64 = 2147483648
	threeGiBBytes int64 = 3221225472
	fourGiBBytes  int64 = 4294967296
	eightGiBBytes int64 = 8589934592

	fixtureCgroupBase = "/sys/fs/cgroup"
)

var errHostUnreadable = errors.New("host memory unreadable")

func bytesText(bytes int64) string {
	return strconv.FormatInt(bytes, 10) + "\n"
}

func cgroupFiles(memoryMax, memoryCurrent string) map[string]string {
	return map[string]string{
		fixtureCgroupBase + "/memory.max":     memoryMax,
		fixtureCgroupBase + "/memory.current": memoryCurrent,
	}
}

func memoryPressureText(someAvg60 string) string {
	return "some avg10=0.00 avg60=" + someAvg60 + " avg300=0.00 total=0\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=0\n"
}

func withFile(files map[string]string, file, content string) map[string]string {
	extended := maps.Clone(files)
	extended[fixtureCgroupBase+"/"+file] = content

	return extended
}

func cgroupV1Files(limitInBytes, usageInBytes string) map[string]string {
	return map[string]string{
		fixtureCgroupBase + "/memory/memory.limit_in_bytes": limitInBytes,
		fixtureCgroupBase + "/memory/memory.usage_in_bytes": usageInBytes,
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
	}).WithFileExistsFunc(func(_ context.Context, path string) (bool, error) {
		_, ok := files[path]

		return ok, nil
	})
}

func hostMemoryOf(usedBytes, totalBytes int64) memoryhealth.HostMemoryReader {
	return func(context.Context) (uint64, uint64, error) {
		return uint64(usedBytes), uint64(totalBytes), nil
	}
}

func unreadableHostMemory(context.Context) (uint64, uint64, error) {
	return 0, 0, errHostUnreadable
}
