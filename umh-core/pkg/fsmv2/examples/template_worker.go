package examples

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/location"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/templating"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
)

type TemplateWorker struct {
	ParentLocation []location.LocationLevel
	ChildLocation  []location.LocationLevel
	Template       string
	MultiChild     bool
}

func NewTemplateWorker(parentLocation, childLocation []location.LocationLevel) *TemplateWorker {
	defaultTemplate := `
input:
  mqtt:
    urls: ["tcp://{{ .IP }}:{{ .PORT }}"]
output:
  kafka:
    topic: "umh.v1.{{ .location_path }}.{{ .name }}"
`

	return &TemplateWorker{
		ParentLocation: parentLocation,
		ChildLocation:  childLocation,
		Template:       defaultTemplate,
		MultiChild:     false,
	}
}

func (w *TemplateWorker) DeriveDesiredState(userSpec types.UserSpec) (types.DesiredState, error) {
	flattened := userSpec.Variables.Flatten()

	mergedLocation := location.MergeLocations(w.ParentLocation, w.ChildLocation)
	filledLocation := location.FillISA95Gaps(mergedLocation)
	locationPath := location.ComputeLocationPath(filledLocation)
	flattened["location_path"] = locationPath

	if w.MultiChild {
		return w.deriveMultiChild(flattened)
	}

	return w.deriveSingleChild(flattened)
}

func (w *TemplateWorker) deriveSingleChild(flattened map[string]any) (types.DesiredState, error) {
	childConfig, err := templating.RenderTemplate(w.Template, flattened)
	if err != nil {
		return types.DesiredState{}, fmt.Errorf("render template: %w", err)
	}

	return types.DesiredState{
		State: "running",
		ChildrenSpecs: []types.ChildSpec{
			{
				Name:       "mqtt_source",
				WorkerType: "benthos_dataflow",
				UserSpec: types.UserSpec{
					Config: childConfig,
					Variables: types.VariableBundle{
						User: map[string]any{
							"name": "mqtt_source",
						},
					},
				},
			},
		},
	}, nil
}

func (w *TemplateWorker) deriveMultiChild(flattened map[string]any) (types.DesiredState, error) {
	mqttTemplate := `
input:
  mqtt:
    urls: ["tcp://{{ .IP }}:{{ .PORT }}"]
`
	kafkaTemplate := `
output:
  kafka:
    topic: "umh.v1.{{ .location_path }}.{{ .name }}"
`

	mqttConfig, err := templating.RenderTemplate(mqttTemplate, flattened)
	if err != nil {
		return types.DesiredState{}, fmt.Errorf("render mqtt template: %w", err)
	}

	kafkaConfig, err := templating.RenderTemplate(kafkaTemplate, flattened)
	if err != nil {
		return types.DesiredState{}, fmt.Errorf("render kafka template: %w", err)
	}

	return types.DesiredState{
		State: "running",
		ChildrenSpecs: []types.ChildSpec{
			{
				Name:       "mqtt_source",
				WorkerType: "benthos_dataflow",
				UserSpec: types.UserSpec{
					Config: mqttConfig,
					Variables: types.VariableBundle{
						User: map[string]any{
							"name": "mqtt_source",
						},
					},
				},
			},
			{
				Name:       "kafka_sink",
				WorkerType: "benthos_dataflow",
				UserSpec: types.UserSpec{
					Config: kafkaConfig,
					Variables: types.VariableBundle{
						User: map[string]any{
							"name": "kafka_sink",
						},
					},
				},
			},
		},
	}, nil
}
