# Stream Processors

> **Important**: Stream processors create different views of existing [Silver data](../data-modeling/README.md#silver-data). Most users don't need them - use [bridges](bridges.md) with [data models](../data-modeling/data-models.md) and [data contracts](../data-modeling/data-contracts.md) instead.
>
> **Current Limitation**: [Time-series data](../unified-namespace/payload-formats.md#time-series-data) only. [Relational output](../unified-namespace/payload-formats.md#relational-data) for business records (work orders, maintenance requests) is planned. For now, use for relational data [standalone flows](stand-alone-flow.md) instead.

Stream processors transform data that's already in the UNS into a different structure or view.

## When You Need Stream Processors

| What You Want | Solution | Why |
|---------------|----------|-----|
| Structure data from one device | [Bridge](bridges.md) + [data model](../data-modeling/data-models.md) | Bridges can write to any model |
| Data from multiple devices in one model | Multiple [bridges](bridges.md) → same model | Each bridge fills different fields |
| **Different view of existing data** | **Stream Processor** | Transform Silver → different structure |
| **Business records (future)** | **Stream Processor** | When relational support is added |

## How Stream Processors Work

Stream processors use templates to define reusable transformations:

```yaml
# Simplified example
payloadshapes:
  timeseries-number:
    fields:
      timestamp_ms:
        _type: number
      value:
        _type: number

datamodels:
  pump:
    description: "Pump with motor sub-model"
    versions:
      v1:
        structure:
          pressure:
            _payloadshape: timeseries-number
          motor:
            _refModel:
              name: motor
              version: v1

datacontracts:
  - name: _pump_v1
    model:
      name: pump
      version: v1
    default_bridges:
      - type: timescaledb
        retention_in_days: 365

templates:
  streamProcessors:
    pump_template:
      model:
        name: pump
        version: v1
      sources:
        press: "${{ .location_path }}._raw.${{ .pressure_sensor }}"
        temp: "${{ .location_path }}._raw.tempF"
      mapping:
        pressure: "press"
        temperature: "(temp-32)*5/9"
        serialNumber: "${{ .sn }}"

streamprocessors:
  - name: pump41_sp
    _templateRef: "pump_template"
    location:
      0: corpA
      1: plant-A
      2: line-4
      3: pump41
    variables:
      pressure_sensor: "pressure"
      sn: "SN-P41-007"
```

## Configuration

### Templates

Define once, use many times with different variables:

```yaml
templates:
  streamProcessors:
    template_name:
      model:
        name: model_name
        version: v1
      sources:
        var_name: "${{ .location_path }}._raw.${{ .variable_name }}"
      mapping:
        model_field: "javascript_expression"
        folder:
          sub_field: "javascript_expression"
        metadata_field: "${{ .variable_name }}"
```

### Stream Processor Instances

```yaml
streamprocessors:
  - name: processor_name
    _templateRef: "template_name"     # Required: references template
    location:                        # Hierarchical organization (ISA-95, KKS, or custom)
      0: enterprise                  # Level 0 (mandatory)
      1: site                        # Level 1 (optional)
      2: area                        # Level 2 (optional)
      3: work_unit                   # Level 3 (optional)
      4: work_center                 # Level 4 (optional)
    variables:                       # Template variable substitution
      variable_name: "value"
```

### Location Hierarchy

Stream processors define their position in the hierarchical organization (commonly based on ISA-95 but adaptable to KKS or custom naming standards). For complete hierarchy structure and rules, see [Topic Convention](../unified-namespace/topic-convention.md).

```yaml
location:
  0: corpA        # Enterprise (mandatory)
  1: plant-A      # Site/Region (optional)
  2: line-4       # Area/Zone (optional)
  3: pump42       # Work Unit (optional)
  4: motor1       # Work Center (optional)
```

This creates UNS topics following the standard convention:
```
umh.v1.{0}.{1}.{2}.{3}[.{4}].{contract}.{field_path}
```

### Variables

Replace placeholders in templates with actual values:

```yaml
templates:
  streamProcessors:
    sensor_template:
      model:
        name: temperature
        version: v1
      sources:
        temp: "${{ .location_path }}._raw.${{ .sensor_name }}"
      mapping:
        temperatureInC: "(temp - 32) * 5 / 9"
        sensor_id: "${{ .sensor_id }}"
        location: "${{ .location_description }}"
```

**Built-in Variables:**
- `${{ .location_path }}`: Auto-generated from location hierarchy (e.g., `umh.v1.corpA.plant-A.line-4.pump41`)

**Custom Variables:**
- Define in `variables:` section of stream processor
- Reference in templates using `${{ .variable_name }}`

### Mapping

Transform source data with JavaScript:

```yaml
mapping:
  # Direct pass-through
  pressure: "press"

  # Unit conversions
  temperature: "(temp - 32) * 5 / 9"

  # Calculations
  total_power: "l1 + l2 + l3"

  # Conditional logic
  status: "temp > 100 ? 'hot' : 'normal'"

  # Sub-model fields (nested YAML)
  motor:
    current: "motor_current_var"
    rpm: "motor_speed_var"

  # Folder fields
  diagnostics:
    vibration: "vibration_var"
```

Or use static values from variables:

```yaml
mapping:
  serialNumber: "${{ .sn }}"
  firmware_version: "${{ .firmware_ver }}"
  installation_date: "${{ .install_date }}"
```

## Example: Creating a Different View

Let's say you have temperature data in Fahrenheit across multiple furnaces (already in Silver as `_raw`), and you want a unified Celsius view:

```yaml
# Template: Convert any Fahrenheit source to Celsius model
templates:
  streamProcessors:
    temp_celsius_converter:
      model:
        name: temperature
        version: v1
      sources:
        tempF: "${{ .location_path }}._raw.temperature_F"
      mapping:
        temperatureInC: "(tempF - 32) * 5 / 9"

# Create different view for each furnace
streamprocessors:
  - name: furnace1_celsius
    _templateRef: "temp_celsius_converter"
    location:
      0: corpA
      1: plant-A
      2: line-4
      3: furnace1

  - name: furnace2_celsius
    _templateRef: "temp_celsius_converter"
    location:
      0: corpA
      1: plant-A
      2: line-4
      3: furnace2
```

Now you have both views:
- Original: `umh.v1.corpA.plant-A.line-4.furnace1._raw.temperature_F` (Fahrenheit)
- New view: `umh.v1.corpA.plant-A.line-4.furnace1._temperature_v1.temperatureInC` (Celsius)

## Management Console

> 🚧 **Roadmap Item** - Visual interface for creating stream processors coming soon.

## Related Documentation

- [Data Modeling Overview](../data-modeling/README.md) - When to use stream processors vs bridges
- [Data Models](../data-modeling/data-models.md) - Structure definition
- [Data Contracts](../data-modeling/data-contracts.md) - Enforcement mechanism
