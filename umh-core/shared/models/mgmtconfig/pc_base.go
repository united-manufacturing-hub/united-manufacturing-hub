package mgmtconfig

import (
	"errors"
	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/ManagementConsole/shared/models/ctypes"
	"time"
)

type ProtocolConverterBase struct {
	Type           ctypes.Protocol `yaml:"type" json:"type"`
	Name           string          `yaml:"name" json:"name"`
	ConnectionUUID uuid.UUID       `yaml:"connectionUUID" json:"connectionUUID"`
	UUID           uuid.UUID       `yaml:"uuid" json:"uuid"`
	// LastUpdated is the unix utc nano timestamp of the last time the protocol converter was updated
	LastUpdated uint64 `yaml:"lastUpdated" json:"lastUpdated"`
	// Metadata is an optional field that can be used to store additional information about the protocol converter
	Metadata map[string]string `yaml:"metadata" json:"metadata"`
}

func (p *ProtocolConverterBase) Validate() error {
	if p.Type == "" {
		return errors.New("type is required")
	}
	if p.ConnectionUUID == uuid.Nil {
		return errors.New("connectionUUID is required")
	}
	if p.Name == "" {
		return errors.New("name is required")
	}
	if p.LastUpdated == 0 {
		return errors.New("lastUpdated is required")
	}
	return nil
}

func (p *ProtocolConverterBase) GetConnectionUUID() uuid.UUID {
	return p.ConnectionUUID
}

func (p *ProtocolConverterBase) GetType() ctypes.Protocol {
	return p.Type
}

func (p *ProtocolConverterBase) GetLastUpdated() uint64 {
	return p.LastUpdated
}

func (p *ProtocolConverterBase) GetName() string {
	return p.Name
}

func (p *ProtocolConverterBase) GetUUID() uuid.UUID {
	return p.UUID
}

func (p *ProtocolConverterBase) GetMetadata() map[string]string {
	// If metadata is nil, return an empty map
	if p.Metadata == nil {
		return make(map[string]string)
	}
	return p.Metadata
}

func (p *ProtocolConverterBase) UpdateMetadata(metadata map[string]string) {
	if metadata != nil {
		return
	}

	// Loop trough the map and upsert the values
	for key, value := range metadata {
		p.Metadata[key] = value
	}
}

func (p *ProtocolConverterBase) SetLastUpdated(lastUpdated time.Time) {
	p.LastUpdated = uint64(lastUpdated.UnixNano())
}
