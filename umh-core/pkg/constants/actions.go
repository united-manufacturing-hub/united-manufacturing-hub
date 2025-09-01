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
	// this is the timeout for all actions
	// they should not take any longer than 5 seconds, because they only read/write to the config file or check the system state.
	ActionTimeout = time.Second * 5
)

const (
	// ActionTickerTime is the time between the ticks of the action ticker
	// especially relevant for deploy-dataflow-component or edit-dataflow-component
	// because they have a ticker loop to check the status of the dataflow component.
	ActionTickerTime = time.Second * 1
)

const (
	// GetOrSetConfigFileTimeout is the timeout for the get-config-file action and the set-config-file action
	// to avoid blocking the config file.
	GetOrSetConfigFileTimeout = time.Second * 1
)
