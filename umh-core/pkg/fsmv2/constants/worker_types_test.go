package constants

import (
	"testing"
)

// TestWorkerTypeConstantsExist verifies all expected constants are defined
func TestWorkerTypeConstantsExist(t *testing.T) {
	// Test that each constant exists and has the correct value for backwards compatibility
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"WorkerTypeChild", WorkerTypeChild, "child"},
		{"WorkerTypeParent", WorkerTypeParent, "parent"},
		{"WorkerTypeMQTTClient", WorkerTypeMQTTClient, "mqtt_client"},
		{"WorkerTypeContainer", WorkerTypeContainer, "container"},
		{"WorkerTypeS6Service", WorkerTypeS6Service, "s6_service"},
		{"WorkerTypeOPCUAClient", WorkerTypeOPCUAClient, "opcua_client"},
		{"WorkerTypeModbusServer", WorkerTypeModbusServer, "modbus_server"},
		{"WorkerTypeBenthos", WorkerTypeBenthos, "benthos"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// TestWorkerTypeConstantsNoDuplicates verifies no duplicate values exist
func TestWorkerTypeConstantsNoDuplicates(t *testing.T) {
	// Collect all constant values
	values := []string{
		WorkerTypeChild,
		WorkerTypeParent,
		WorkerTypeMQTTClient,
		WorkerTypeContainer,
		WorkerTypeS6Service,
		WorkerTypeOPCUAClient,
		WorkerTypeModbusServer,
		WorkerTypeBenthos,
	}

	// Check for duplicates
	seen := make(map[string]string)
	for i, v := range values {
		names := []string{
			"WorkerTypeChild",
			"WorkerTypeParent",
			"WorkerTypeMQTTClient",
			"WorkerTypeContainer",
			"WorkerTypeS6Service",
			"WorkerTypeOPCUAClient",
			"WorkerTypeModbusServer",
			"WorkerTypeBenthos",
		}
		if existing, ok := seen[v]; ok {
			t.Errorf("duplicate value %q: %s and %s", v, existing, names[i])
		}
		seen[v] = names[i]
	}
}

// TestWorkerTypeConstantsNotEmpty verifies no constants are empty strings
func TestWorkerTypeConstantsNotEmpty(t *testing.T) {
	constants := map[string]string{
		"WorkerTypeChild":        WorkerTypeChild,
		"WorkerTypeParent":       WorkerTypeParent,
		"WorkerTypeMQTTClient":   WorkerTypeMQTTClient,
		"WorkerTypeContainer":    WorkerTypeContainer,
		"WorkerTypeS6Service":    WorkerTypeS6Service,
		"WorkerTypeOPCUAClient":  WorkerTypeOPCUAClient,
		"WorkerTypeModbusServer": WorkerTypeModbusServer,
		"WorkerTypeBenthos":      WorkerTypeBenthos,
	}

	for name, value := range constants {
		if value == "" {
			t.Errorf("%s is empty", name)
		}
	}
}

// TestAllWorkerTypesSlice verifies the AllWorkerTypes slice contains all types
func TestAllWorkerTypesSlice(t *testing.T) {
	expectedTypes := map[string]bool{
		"child":         false,
		"parent":        false,
		"mqtt_client":   false,
		"container":     false,
		"s6_service":    false,
		"opcua_client":  false,
		"modbus_server": false,
		"benthos":       false,
	}

	// Mark found types
	for _, wt := range AllWorkerTypes {
		if _, exists := expectedTypes[wt]; !exists {
			t.Errorf("unexpected worker type in AllWorkerTypes: %q", wt)
		}
		expectedTypes[wt] = true
	}

	// Check all expected types were found
	for wt, found := range expectedTypes {
		if !found {
			t.Errorf("missing worker type in AllWorkerTypes: %q", wt)
		}
	}

	// Verify count matches
	if len(AllWorkerTypes) != len(expectedTypes) {
		t.Errorf("AllWorkerTypes has %d elements, expected %d", len(AllWorkerTypes), len(expectedTypes))
	}
}
