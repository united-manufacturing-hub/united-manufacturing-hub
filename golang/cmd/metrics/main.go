// Copyright 2023 UMH Systems GmbH
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

package main

import (
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/mem"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"go.uber.org/zap"
	"golang.org/x/crypto/sha3"
	"runtime"
	"strings"
)

type stat struct {
	OS      string
	Arch    string
	Memory  *mem.VirtualMemoryStat
	CPUInfo []cpu.InfoStat
	Host    *host.InfoStat
	Load    *load.AvgStat
}

func main() {
	// Initialize zap logging
	log := logger.New("LOGGING_LEVEL")
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

	// Get OS and architecture
	os := runtime.GOOS
	arch := runtime.GOARCH

	// Get total memory
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		zap.S().Warnf("error: %s", err)
	}

	cpuInfo, err := cpu.Info()
	if err != nil {
		zap.S().Warnf("error: %s", err)
	}

	hostInfo, err := host.Info()
	if err != nil {
		zap.S().Warnf("error: %s", err)
	}

	loadInfo, err := load.Avg()
	if err != nil {
		zap.S().Warnf("error: %s", err)
	}

	// remove PII
	hostNameHasher := sha3.New512()
	hostNameHasher.Write([]byte(hostInfo.Hostname))
	hostInfo.Hostname = fmt.Sprintf("%x", hostNameHasher.Sum(nil))

	hostIdHasher := sha3.New512()
	hostIdHasher.Write([]byte(hostInfo.HostID))
	hostInfo.HostID = fmt.Sprintf("%x", hostIdHasher.Sum(nil))

	// Strip tailing whitespace from CPUInfo.modelName
	for i := 0; i < len(cpuInfo); i++ {
		cpuInfo[i].ModelName = strings.Trim(cpuInfo[i].ModelName, " ")
	}

	s := stat{
		OS:      os,
		Arch:    arch,
		Memory:  vmStat,
		CPUInfo: cpuInfo,
		Host:    hostInfo,
		Load:    loadInfo,
	}

	// JSON serialization
	jsonMetrics, err := jsoniter.MarshalIndent(s, "", "  ")
	if err != nil {
		zap.S().Errorf("error: %s", err)
		return
	}

	zap.S().Infof("Metrics: %s", jsonMetrics)
}
