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

package constants

// Container monitor constants
const (
	// Thresholds for determining critical health status
	CPUCriticalPercent    = 90.0
	MemoryCriticalPercent = 90.0
	DiskCriticalPercent   = 90.0

	// Hardware information sources
	HWIDFilePath  = "/data/hwid"
	DataMountPath = "/data"
)

// Health state constants
// TODO: put in fsm package
const (
	// Health states aligned with generator.go
	ContainerStateNormal   = "normal"
	ContainerStateWarning  = "warning"
	ContainerStateCritical = "critical"
	ContainerStateRunning  = "running"
)
