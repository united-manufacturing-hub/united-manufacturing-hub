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

package version

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// AppVersion is set during build time via ldflags.
// IMPORTANT: This should not be set manually, it will be filled in by the build system.
var AppVersion = constants.DefaultAppVersion

// GetAppVersion returns the current application version string.
func GetAppVersion() string {
	return AppVersion
}

// GetComponentVersions returns all the component versions as defined
// by the constants package, populated during build by the build system.
func GetComponentVersions() []models.Version {
	return []models.Version{
		{
			Name:    "UMH Core",
			Version: AppVersion,
		},
		{
			Name:    "S6 Overlay",
			Version: constants.S6OverlayVersion,
		},
		{
			Name:    "Benthos",
			Version: constants.BenthosVersion,
		},
		{
			Name:    "Redpanda",
			Version: constants.RedpandaVersion,
		},
	}
}
