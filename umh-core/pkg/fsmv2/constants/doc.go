// Package constants defines type-safe constants for the FSMv2 package.
//
// This package provides centralized constants to replace magic strings
// throughout the FSMv2 codebase, improving type safety and maintainability.
//
// # Worker Types
//
// Worker type constants are used for factory registration and identification:
//
//	import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/constants"
//
//	// Register a factory with type-safe constant
//	err := factory.RegisterFactoryByType(constants.WorkerTypeMQTTClient, factoryFunc)
//
//	// Use in child spec definitions
//	spec := config.ChildSpec{
//	    Name:       "mqtt-connection",
//	    WorkerType: constants.WorkerTypeMQTTClient,
//	}
//
// # Available Constants
//
// The following worker types are defined:
//   - WorkerTypeChild: Generic child workers
//   - WorkerTypeParent: Parent/supervisor workers
//   - WorkerTypeMQTTClient: MQTT client workers
//   - WorkerTypeContainer: Container workers
//   - WorkerTypeS6Service: S6 service workers
//   - WorkerTypeOPCUAClient: OPC UA client workers
//   - WorkerTypeModbusServer: Modbus server workers
//   - WorkerTypeBenthos: Benthos workers
//
// # Backwards Compatibility
//
// All constant values match existing string literals to maintain
// backwards compatibility with existing configurations and storage.
package constants
