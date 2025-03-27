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

package logger

// Component name constants for standardized logging
const (
	// Core components
	ComponentCore              = "Core"
	ComponentControlLoop       = "ControlLoop"
	ComponentStarvationChecker = "StarveCheck"

	// Manager components
	ComponentS6Manager      = "S6Manager"
	ComponentBenthosManager = "BenthosManager"

	// FSM components
	ComponentBaseFSM         = "BaseFSM"
	ComponentS6Instance      = "S6Instance"
	ComponentBenthosInstance = "BenthosInstance"

	// Service components
	ComponentS6Service      = "S6Service"
	ComponentBenthosService = "BenthosService"

	// Configuration
	ComponentConfigManager = "ConfigManager"
)
