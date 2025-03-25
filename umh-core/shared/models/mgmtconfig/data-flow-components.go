package mgmtconfig

import (
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models/generated"
)

type DataFlowComponents map[uuid.UUID]DataFlowComponent

type DataFlowComponent struct {
	Internal DataFlowInternal                       `json:"internal" yaml:"internal"`
	Versions map[uuid.UUID]DataFlowComponentVersion `json:"versions" yaml:"versions"`
}

type DataFlowInternal struct {
	LastUpdated     time.Time `json:"lastUpdated" yaml:"lastUpdated"`
	DeployedVersion uuid.UUID `json:"deployedVersion" yaml:"deployedVersion"`
}

type DataFlowComponentVersion struct {
	IsExternal   bool   `json:"isExternal" yaml:"isExternal"`
	CreationTime uint64 `json:"creationTime" yaml:"creationTime"`
	// Below is only set if IsExternal is false
	Name    *string                                `json:"name" yaml:"name,omitempty"`
	Creator *string                                `json:"creator" yaml:"creator,omitempty"` // Identified by email
	Meta    *generated.CommonDataFlowComponentMeta `json:"meta" yaml:"meta,omitempty"`

	DeploySuccess bool `json:"deploySuccess" yaml:"deploySuccess"`

	// This can be one of the following types
	// - generated.CommonDataFlowComponentCDFCProperties
	// - generated.CommonDataFlowComponentProtocolConverterProperties
	// - generated.CommonDataFlowComponentStreamProcessorProperties
	// - generated.CommonDataFlowComponentBridgeProperties
	Config generated.CommonDataFlowComponentOneOfDFCTypes `json:"config" yaml:"config,omitempty"`
}
