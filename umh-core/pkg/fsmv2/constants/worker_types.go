package constants

// WorkerType constants for factory registration and identification.
// These values must match existing string literals for backwards compatibility.
const (
	// WorkerTypeChild is the type identifier for generic child workers.
	WorkerTypeChild = "child"

	// WorkerTypeParent is the type identifier for parent/supervisor workers.
	WorkerTypeParent = "parent"

	// WorkerTypeMQTTClient is the type identifier for MQTT client workers.
	WorkerTypeMQTTClient = "mqtt_client"

	// WorkerTypeContainer is the type identifier for container workers.
	WorkerTypeContainer = "container"

	// WorkerTypeS6Service is the type identifier for S6 service workers.
	WorkerTypeS6Service = "s6_service"

	// WorkerTypeOPCUAClient is the type identifier for OPC UA client workers.
	WorkerTypeOPCUAClient = "opcua_client"

	// WorkerTypeModbusServer is the type identifier for Modbus server workers.
	WorkerTypeModbusServer = "modbus_server"

	// WorkerTypeBenthos is the type identifier for Benthos workers.
	WorkerTypeBenthos = "benthos"
)

// AllWorkerTypes contains all defined worker types.
// Useful for validation and iteration.
var AllWorkerTypes = []string{
	WorkerTypeChild,
	WorkerTypeParent,
	WorkerTypeMQTTClient,
	WorkerTypeContainer,
	WorkerTypeS6Service,
	WorkerTypeOPCUAClient,
	WorkerTypeModbusServer,
	WorkerTypeBenthos,
}
