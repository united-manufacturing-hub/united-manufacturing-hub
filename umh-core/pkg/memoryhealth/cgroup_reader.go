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
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// CgroupMemory is a cgroup's memory limit and current usage.
type CgroupMemory struct {
	LimitBytes   int64
	CurrentBytes int64
	Unlimited    bool
}

// ReadOutcome names why a cgroup memory file could not be read.
type ReadOutcome string

const (
	ReadMissing    ReadOutcome = "missing"
	ReadUnparsable ReadOutcome = "unparsable"
	ReadFailed     ReadOutcome = "failed"
)

// CgroupReadError names the cgroup memory file that could not be read and why.
type CgroupReadError struct {
	File    string
	Outcome ReadOutcome
	Err     error
}

func (e *CgroupReadError) Error() string {
	return fmt.Sprintf("cgroup %s %s: %v", e.File, e.Outcome, e.Err)
}

func (e *CgroupReadError) Unwrap() error {
	return e.Err
}

type cgroupReader interface {
	read(ctx context.Context) (CgroupMemory, error)
}

func readCgroupFile(ctx context.Context, fileSystem filesystem.Service, cgroupBase, file string) ([]byte, error) {
	data, err := fileSystem.ReadFile(ctx, cgroupBase+"/"+file)
	if err == nil {
		return data, nil
	}

	outcome := ReadFailed
	if errors.Is(err, fs.ErrNotExist) {
		outcome = ReadMissing
	}

	return nil, &CgroupReadError{File: file, Outcome: outcome, Err: err}
}

func readByteCountFile(ctx context.Context, fileSystem filesystem.Service, cgroupBase, file string) (int64, error) {
	data, err := readCgroupFile(ctx, fileSystem, cgroupBase, file)
	if err != nil {
		return 0, err
	}

	byteCount, err := parseByteCount(file, string(data))
	if err != nil {
		return 0, &CgroupReadError{File: file, Outcome: ReadUnparsable, Err: err}
	}

	return byteCount, nil
}

func parseByteCount(file, text string) (int64, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0, fmt.Errorf("empty %s data", file)
	}

	byteCount, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s value %q: %w", file, trimmed, err)
	}

	if byteCount < 0 {
		return 0, fmt.Errorf("negative %s value %q", file, trimmed)
	}

	return byteCount, nil
}
