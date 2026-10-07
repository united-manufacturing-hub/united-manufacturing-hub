---
paths:
  - "umh-core/pkg/config/**"
  - "umh-core/pkg/service/protocolconverter/**"
  - "umh-core/pkg/service/streamprocessor/**"
  - "umh-core/examples/**"
---

# Templates and variables in config.yaml

The Management Console never writes benthos config. It changes `config.yaml`. umh-core renders each bridge or stream processor from its template and variables.

Worked example: `umh-core/examples/example-config-protocolconverter-templated.yaml` (unit tests depend on it; do not change it without the tests).

## Shape

```yaml
templates:
  protocolConverter:
    temperature-sensor-pc:            # template name
      connection:
        nmap:
          target: '{{ .IP }}'
          port: '{{ .PORT }}'
      dataflowcomponent_read:
        benthos:
          input:
            opcua:
              address: "opc.tcp://{{ .IP }}:{{ .PORT }}"

agent:
  location:                           # authoritative; a bridge cannot override these levels
    0: "plant-A"
    1: "line-4"

protocolConverter:
  - name: temperature-sensor-pc
    desiredState: active
    protocolConverterServiceConfig:
      location:
        2: "machine-7"                # adds a level below the agent's
      templateRef: "temperature-sensor-pc"   # null = use the inline `config:`
      variables:                      # flat, user-defined
        IP: "10.0.1.50"
        PORT: "4840"
```

## Rules

- **Flattening**: user variables become top-level template keys: `{{ .IP }}`, not `{{ .variables.IP }}`. Global and internal variables stay under `{{ .global.* }}` and `{{ .internal.* }}` (`VariableBundle.Flatten` in `pkg/config/variables/variables.go`).
- **Location**: the agent location wins on every level it sets. Missing levels up to the highest one set are filled with `"unknown"`. `{{ .location_path }}` is the levels joined with `.` (`pkg/service/protocolconverter/runtime_config/runtime_config.go`; stream processors in the matching `streamprocessor` file).
- `location`, `location_path` and `historian` are reserved variable names. A user variable with one of these names is overwritten.
- `templates:` exists only in the YAML on disk. Template resolution (`convertYamlToSpec` in `pkg/config/yamlParsing.go`) materialises every instance and clears `Templates` in the in-memory config.
- Rendering: `RenderTemplate` in `pkg/config/templating.go`.
