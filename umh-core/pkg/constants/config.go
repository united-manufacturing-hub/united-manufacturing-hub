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

import "time"

const (
	// ConfigGetConfigTimeout defines the maximum time allowed for retrieving configurations.
	// This timeout (20ms) prevents configuration retrieval from blocking the reconciliation
	ConfigGetConfigTimeout = time.Millisecond * 20

	// AmountReadersForConfigFile defines the amount of readers that can read the config file at the same time
	// It is more a safety net to prevent a single reader from blocking the config file
	// The actual number does not really matter, it should be "high enough"
	AmountReadersForConfigFile = 100
)
