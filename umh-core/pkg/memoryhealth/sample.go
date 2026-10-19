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
	"time"

	"github.com/shirou/gopsutil/v4/mem"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// Source names what a sample's used and total bytes were read from.
type Source string

const (
	SourceCgroupLimit                  Source = "cgroup_limit"
	SourceHostTotalNoCgroupLimit       Source = "host_total_no_cgroup_limit"
	SourceHostFallbackCgroupUnreadable Source = "host_fallback_cgroup_unreadable"
)

// ErrZeroTotal is returned for a sample whose total memory is zero.
var ErrZeroTotal = errors.New("memory total is zero")

// HostMemoryReader returns the machine's used and total memory.
type HostMemoryReader func(ctx context.Context) (usedBytes, totalBytes uint64, err error)

// Sample is one reading of the container's memory.
type Sample struct {
	Timestamp     time.Time
	Source        Source
	UsedBytes     int64
	TotalBytes    int64
	CgroupReadErr error
	HostReadErr   error
}

// Sampler reads the cgroup's memory and falls back to the host's memory where
// the cgroup is unreadable or has no limit.
type Sampler struct {
	fileSystem filesystem.Service
	cgroupBase string
	hostMemory HostMemoryReader
}

// NewSampler returns a Sampler reading the cgroup under cgroupBase.
func NewSampler(fileSystem filesystem.Service, cgroupBase string, hostMemory HostMemoryReader) *Sampler {
	return &Sampler{fileSystem: fileSystem, cgroupBase: cgroupBase, hostMemory: hostMemory}
}

// ReadHostMemory returns the machine's used and total memory from the operating
// system.
func ReadHostMemory(ctx context.Context) (usedBytes, totalBytes uint64, err error) {
	virtualMemory, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return 0, 0, err
	}

	return virtualMemory.Used, virtualMemory.Total, nil
}

// Read takes one sample. It returns the sample with an error when no total
// could be measured; the sample's read errors then name what failed.
func (s *Sampler) Read(ctx context.Context) (Sample, error) {
	cgroup, cgroupErr := ReadCgroupMemory(ctx, s.fileSystem, s.cgroupBase)
	if ctx.Err() != nil {
		return Sample{}, ctx.Err()
	}

	sample, err := s.chooseSource(ctx, Sample{Timestamp: time.Now(), CgroupReadErr: cgroupErr}, cgroup)
	if err != nil {
		return sample, err
	}

	if sample.TotalBytes <= 0 {
		return sample, ErrZeroTotal
	}

	return sample, nil
}

func (s *Sampler) chooseSource(ctx context.Context, sample Sample, cgroup CgroupMemory) (Sample, error) {
	if sample.CgroupReadErr != nil {
		return s.hostFallback(ctx, sample)
	}

	if cgroup.Unlimited || cgroup.LimitBytes == 0 {
		return s.cgroupAgainstHostTotal(ctx, sample, cgroup.CurrentBytes)
	}

	sample.Source = SourceCgroupLimit
	sample.UsedBytes = cgroup.CurrentBytes
	sample.TotalBytes = cgroup.LimitBytes

	return sample, nil
}

func (s *Sampler) hostFallback(ctx context.Context, sample Sample) (Sample, error) {
	hostUsed, hostTotal, hostErr := s.hostMemory(ctx)
	if hostErr != nil {
		sample.HostReadErr = hostErr

		return sample, fmt.Errorf("cgroup memory is unreadable (%w) and host memory is unreadable: %w", sample.CgroupReadErr, hostErr)
	}

	sample.Source = SourceHostFallbackCgroupUnreadable
	sample.UsedBytes = int64(hostUsed)
	sample.TotalBytes = int64(hostTotal)

	return sample, nil
}

func (s *Sampler) cgroupAgainstHostTotal(ctx context.Context, sample Sample, cgroupUsedBytes int64) (Sample, error) {
	_, hostTotal, hostErr := s.hostMemory(ctx)
	if hostErr != nil {
		sample.HostReadErr = hostErr

		return sample, fmt.Errorf("cgroup memory has no limit and host memory is unreadable: %w", hostErr)
	}

	sample.Source = SourceHostTotalNoCgroupLimit
	sample.UsedBytes = cgroupUsedBytes
	sample.TotalBytes = int64(hostTotal)

	return sample, nil
}
