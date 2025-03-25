package mgmtconfig

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/ManagementConsole/shared/models/ctypes"
	"gopkg.in/yaml.v3"
	"time"

	"go.uber.org/zap"
)

type ProtocolConverter interface {
	Validate() error
	ToYAML() ([]byte, error)
	GetType() ctypes.Protocol
	GetConnectionUUID() uuid.UUID
	GetLastUpdated() uint64
	GetName() string
	GetUUID() uuid.UUID
	GetMetadata() map[string]string
	UpdateMetadata(metadata map[string]string)
	SetLastUpdated(now time.Time)
	GetCopy() (ProtocolConverter, error)
}

type ProtocolConverterMap map[uuid.UUID]ProtocolConverter

func ProtocolConverterMapToBinary(data ProtocolConverterMap) ([]byte, error) {
	var yamlMap = make(map[uuid.UUID]string)

	for u, converter := range data {
		yamlData, err := converter.ToYAML()
		if err != nil {
			return nil, err
		}
		yamlMap[u] = string(yamlData)
	}

	fullYaml, err := yaml.Marshal(yamlMap)
	if err != nil {
		return nil, err
	}
	return fullYaml, nil
}

func ProtocolConverterMapFromBinary(data []byte) (ProtocolConverterMap, error) {
	var yamlMap = make(map[uuid.UUID]string)
	err := yaml.Unmarshal(data, &yamlMap)
	if err != nil {
		return nil, err
	}

	var pcm = make(ProtocolConverterMap)

	for _, yamlData := range yamlMap {
		// First we need to unmarshal to map[string]interface{} to get the protocolconverterbase
		var unmarshalled map[string]interface{}
		err := yaml.Unmarshal([]byte(yamlData), &unmarshalled)
		if err != nil {
			return nil, err
		}
		pcbRaw, ok := unmarshalled["protocolconverterbase"]
		if !ok {
			return nil, fmt.Errorf("no protocolconverterbase found in unmarshalled data")
		}
		pcb, ok := pcbRaw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("protocolconverterbase is not a map")
		}

		pTypeRaw, ok := pcb["type"]
		if !ok {
			return nil, fmt.Errorf("no type found in protocolconverterbase")
		}

		pTypeS, ok := pTypeRaw.(string)
		if !ok {
			return nil, fmt.Errorf("type is not a string")
		}

		pType := ctypes.Protocol(pTypeS)

		switch pType {
		case ctypes.BenthosOPCUA:
			zap.S().Warnf("OPC UA protocol-converters are no longer supported, please re-created it as a data flow component. If you need assistance, please contact our support team")
			continue
		case ctypes.BenthosGeneric:
			zap.S().Warnf("Generic protocol-converters are no longer supported, please re-created it as a data flow component. If you need assistance, please contact our support team")
			continue
		default:
			zap.S().Warnf("Unknown protocol type: %s", pType)
			continue
		}
	}

	return pcm, nil
}
