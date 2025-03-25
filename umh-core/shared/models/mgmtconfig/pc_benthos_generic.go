package mgmtconfig

import (
	"errors"
	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/ManagementConsole/shared/models/ctypes"
	"gopkg.in/yaml.v3"
	"time"
)

type ProtocolBenthosGeneric struct {
	ProtocolConverterBase

	InputYaml     map[string]interface{} `yaml:"inputYaml" json:"inputYaml"`
	ProcessorYaml map[string]interface{} `yaml:"processorYaml" json:"processorYaml"`
	InjectYaml    map[string]interface{} `yaml:"injectYaml" json:"injectYaml"`
}

func (p *ProtocolBenthosGeneric) GetType() ctypes.Protocol {
	return p.ProtocolConverterBase.GetType()
}
func (p *ProtocolBenthosGeneric) GetConnectionUUID() uuid.UUID {
	return p.ProtocolConverterBase.GetConnectionUUID()
}
func (p *ProtocolBenthosGeneric) GetLastUpdated() uint64 {
	return p.ProtocolConverterBase.GetLastUpdated()
}
func (p *ProtocolBenthosGeneric) GetName() string {
	return p.ProtocolConverterBase.GetName()
}
func (p *ProtocolBenthosGeneric) GetUUID() uuid.UUID {
	return p.ProtocolConverterBase.GetUUID()
}
func (p *ProtocolBenthosGeneric) GetMetadata() map[string]string {
	return p.ProtocolConverterBase.GetMetadata()
}
func (p *ProtocolBenthosGeneric) UpdateMetadata(metadata map[string]string) {
	p.ProtocolConverterBase.UpdateMetadata(metadata)
}

func (p *ProtocolBenthosGeneric) SetLastUpdated(lastUpdated time.Time) {
	p.ProtocolConverterBase.SetLastUpdated(lastUpdated)
}

func (p *ProtocolBenthosGeneric) ToYAML() ([]byte, error) {
	return yaml.Marshal(p)
}

func (p *ProtocolBenthosGeneric) Validate() error {
	if err := p.ProtocolConverterBase.Validate(); err != nil {
		return err
	}

	if p.InputYaml == nil {
		return errors.New("inputYaml is required")
	}
	if p.ProcessorYaml == nil {
		return errors.New("processorYaml is required")
	}
	return nil
}

func (p *ProtocolBenthosGeneric) GetCopy() (ProtocolConverter, error) {
	// Copy self using serialization
	serialized, err := yaml.Marshal(p)
	if err != nil {
		return nil, err
	}

	// Create new object and deserialize into it
	cp := ProtocolBenthosGeneric{}
	err = yaml.Unmarshal(serialized, &cp)

	return &cp, err
}
