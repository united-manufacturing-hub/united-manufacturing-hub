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

package mgmtconfig

type ReleaseChannel string

const (
	Unset      = ""
	Enterprise = "enterprise"
	Stable     = "stable"
	Nightly    = "nightly"
)

type UmhConfig struct {

	// DisableHardwareStatusCheck is a flag to disable the hardware status check inside the status message
	// It is used for testing purposes
	DisableHardwareStatusCheck *bool          `yaml:"disableHardwareStatusCheck"`
	Version                    string         `yaml:"version"`
	HelmChartAddon             string         `yaml:"helm_chart_addon"`
	ReleaseChannel             ReleaseChannel `yaml:"releaseChannel"`

	UmhMergePoint         int    `yaml:"umh_merge_point"`
	LastUpdated           uint64 `yaml:"lastUpdated"`
	Enabled               bool   `yaml:"enabled"`
	ConvertingActionsDone bool   `yaml:"convertingActionsDone"`
}
