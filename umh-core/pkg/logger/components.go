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
	ComponentFilesystem        = "Filesystem"

	// Manager components
	ComponentS6Manager                = "S6Manager"
	ComponentBenthosManager           = "BenthosManager"
	ComponentBenthosMonitorManager    = "BenthosMonitorManager"
	ComponentRedpandaMonitorManager   = "RedpandaMonitorManager"
	ComponentAgentManager             = "AgentManager"
	ComponentContainerManager         = "ContainerManager"
	ComponentRedpandaManager          = "RedpandaManager"
	ComponentDataFlowComponentManager = "DataFlowCompManager"
	ComponentNmapManager              = "NmapCompManager"
	ComponentConnectionManager        = "ConnectionManager"
	ComponentProtocolConverterManager = "ProtocolConverterManager"
	ComponentStreamProcessorManager   = "StreamProcessorManager"
	ComponentTopicBrowserManager      = "TopicBrowserManager"

	// FSM components
	ComponentBaseFSM                   = "BaseFSM"
	ComponentS6Instance                = "S6Instance"
	ComponentBenthosInstance           = "BenthosInstance"
	ComponentBenthosMonitorInstance    = "BenthosMonitorInstance"
	ComponentRedpandaInstance          = "RedpandaInstance"
	ComponentRedpandaMonitorInstance   = "RedpandaMonitorInstance"
	ComponentNmapInstance              = "NmapInstance"
	ComponentDataFlowComponentInstance = "DFCInstance"
	ComponentConnectionInstance        = "ConnectionInstance"
	ComponentProtocolConverterInstance = "ProtocolConverterInstance"
	ComponentStreamProcessorInstance   = "StreamProcessorInstance"
	ComponentTopicBrowserInstance      = "TopicBrowserInstance"

	// Service components
	ComponentS6Service                = "S6Service"
	ComponentBenthosService           = "BenthosService"
	ComponentBenthosMonitorService    = "BenthosMonitorService"
	ComponentNmapService              = "NmapService"
	ComponentContainerMonitorService  = "ContainerMonitorService"
	ComponentRedpandaMonitorService   = "RedpandaMonitorService"
	ComponentRedpandaService          = "RedpandaService"
	ComponentAgentMonitorService      = "AgentMonitorService"
	ComponentDataFlowComponentService = "DFCService"
	ComponentConnectionService        = "ConnectionService"
	ComponentProtocolConverterService = "ProtocolConverterService"
	ComponentStreamProcessorService   = "StreamProcessorService"
	ComponentFilesystemService        = "FilesystemService"
	ComponentTopicBrowserService      = "TopicBrowserService"
	// Configuration
	ComponentConfigManager = "ConfigManager"

	// Agent
	AgentManagerComponentName  = "AgentManager"
	AgentInstanceComponentName = "agent"

	// Communicator components
	ComponentCommunicator = "Communicator"
)
