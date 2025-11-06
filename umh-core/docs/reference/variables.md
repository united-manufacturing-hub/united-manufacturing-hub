# Template Variables Reference

<a id="variables"></a>

## Overview

UMH Core uses a three-tier variable system for protocol converter templates that enables flexible configuration and runtime customization:

- **User Variables**: Defined in the `variables:` section of your YAML configuration, flattened to top-level access
- **Internal Variables**: Runtime-injected system metadata and computed values  
- **Global Variables**: Fleet-wide settings (**NOT YET IMPLEMENTED**)

## Variable Precedence

Variables follow a clear precedence hierarchy:
1. **User Variables** (highest priority) - Override all others when flattened by VariableBundle
2. **Internal Variables** - System-generated values
3. **Global Variables** - Fleet-wide defaults (**NOT YET IMPLEMENTED**)

## Available Variables

### Connection Variables

These variables provide device connection information, typically auto-injected from Connection configurations or defined by users.

| Variable | Description | Example Value | Source | Status |
|----------|-------------|---------------|---------|---------|
| `{{ .IP }}` | Device IP address or hostname | `192.168.1.100` | User/Connection | Current |
| `{{ .PORT }}` | Device communication port | `4840` | User/Connection | Current |

**Usage Example:**
```yaml
protocolConverter:
  config: |
    input:
      opcua:
        endpoint: "opc.tcp://{{ .IP }}:{{ .PORT }}/OPCUA/SimulationServer"
```

### Location Variables

These variables provide hierarchical location information for data organization and topic routing.

| Variable | Description | Example Value | Source | Status |
|----------|-------------|---------------|---------|---------|
| `{{ .location_path }}` | Dot-separated hierarchical path | `factory.line1.machine1` | Internal | Current |
| `{{ .location }}` | Location as numbered map | `{"0":"factory","1":"line1","2":"machine1"}` | Internal | Current |
| `{{ .location.0 }}` | Enterprise level (first location) | `factory` | Internal | Current |
| `{{ .location.1 }}` | Site level (second location) | `line1` | Internal | Current |
| `{{ .location.2 }}` | Area level (third location) | `machine1` | Internal | Current |

**Usage Example:**
```yaml
protocolConverter:
  config: |
    output:
      mqtt:
        topic: "umh/v1/{{ .location_path }}/data"
        # Or access individual levels:
        # topic: "umh/v1/{{ .location.0 }}/{{ .location.1 }}/data"
```

### Internal System Variables

These variables provide runtime metadata and system integration information.

| Variable | Description | Example Value | Source | Status |
|----------|-------------|---------------|---------|---------|
| `{{ .internal.bridged_by }}` | Bridge identifier that created this config | `protocol-converter-node1-pc123` | Internal | Current |
| `{{ .internal.id }}` | Protocol converter instance ID | `pc-123` | Internal | Current |
| `{{ .internal.umh_topic }}` | UMH topic for write flows | `umh/v1/factory/line1/_historian` | Internal | Current (write flows only) |

**Usage Example:**
```yaml
protocolConverter:
  config: |
    output:
      mqtt:
        topic: "{{ .internal.umh_topic }}"
        metadata:
          bridged_by: "{{ .internal.bridged_by }}"
```

### Global Variables (Not Yet Implemented)

These variables will provide fleet-wide configuration when implemented.

| Variable | Description | Example Value | Source | Status |
|----------|-------------|---------------|---------|---------|
| `{{ .global.cluster_id }}` | Multi-cluster identifier | `cluster-west-1` | Global | **NOT IMPLEMENTED** |
| `{{ .global.* }}` | Other fleet-wide variables | varies | Global | **NOT IMPLEMENTED** |

## Deprecated Variables

The following variables should be replaced in your configurations:

| Deprecated Variable | Status | Replacement | Action Required |
|-------------------|---------|-------------|-----------------|
| `{{ .HOST }}` | Deprecated | `{{ .IP }}` | Replace in all configurations |
| `{{ .DEVICE_IP }}` | Inconsistent | `{{ .IP }}` | Standardize usage |
| `{{ .DEVICE_PORT }}` | Inconsistent | `{{ .PORT }}` | Standardize usage |

## User-Defined Variables

You can define custom variables in the `variables:` section of your configuration. These are flattened to top-level access and override any internal variables with the same name.

**Common patterns:**
- `{{ .SCAN_RATE }}` - Polling intervals
- `{{ .TAG_PREFIX }}` - Tag naming prefixes
- `{{ .USERNAME }}` - Authentication credentials
- `{{ .PASSWORD }}` - Authentication credentials
- `{{ .ADDRESSES }}` - Array of device addresses (S7, Modbus, etc.)

**Example with scalar variables:**
```yaml
protocolConverter:
  variables:
    SCAN_RATE: "1000ms"
    TAG_PREFIX: "PLC1_"
  config: |
    input:
      opcua:
        endpoint: "opc.tcp://{{ .IP }}:{{ .PORT }}/OPCUA/SimulationServer"
        poll_interval: "{{ .SCAN_RATE }}"
        tag_prefix: "{{ .TAG_PREFIX }}"
```

**Example with array variables:**
```yaml
protocolConverter:
  variables:
    ADDRESSES: [DB3.X0.0, DB3.X0.1, DB3.X0.2, DB3.X1.0]
    SLAVE_IDS: [1, 2, 3, 5, 8]
  config: |
    input:
      s7:
        address_list: {{ .ADDRESSES }}
    # Or for Modbus:
    input:
      modbus:
        slave_ids: {{ .SLAVE_IDS }}
```

## Variable Sources

Understanding where variables come from helps with troubleshooting:

- **Connection Variables** (`{{ .IP }}`, `{{ .PORT }}`): Auto-injected from Connection configurations attached to Bridges
- **Location Variables** (`{{ .location_path }}`, `{{ .location }}.*`): Computed from agent location + bridge location by BuildRuntimeConfig
- **Internal Variables** (`{{ .internal.* }}`): Runtime-injected system metadata
- **User Variables**: Explicitly defined in `variables:` section
- **Global Variables**: Fleet-wide settings (**NOT YET IMPLEMENTED**)

## Troubleshooting

### Variable Not Found
If a variable is not resolving:
1. Check spelling and case sensitivity
2. Verify the variable is defined in the `variables:` section (for user variables)
3. Ensure Connection is properly attached (for `.IP`/`.PORT`)
4. Check that location path is configured (for location variables)

### Deprecated Variable Warnings
Replace deprecated variables with their current equivalents:
- `{{ .HOST }}` → `{{ .IP }}`
- `{{ .DEVICE_IP }}` → `{{ .IP }}`
- `{{ .DEVICE_PORT }}` → `{{ .PORT }}`

For questions about variables not listed here, check the [configuration reference](configuration-reference.md) or consult the UMH Core documentation.