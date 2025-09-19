# Topic Convention

> **Prerequisite:** Understand the [UNS concept](README.md) and see it in action via [Getting Started](../../getting-started/).

Every piece of data in the UNS has a unique address that tells you exactly where it came from and what it represents.

## See It In Action

Here's how real PLC data appears in the Topic Browser:

![Topic Browser showing DB1.DW20](../../getting-started/images/2-topic-browser-DB1.DW20.png)

The topic `umh.v1.demo.DefaultArea.DefaultProductionLine.SIEMENS-S7._raw.DB1.DW20` tells us:
- **WHERE**: demo → DefaultArea → DefaultProductionLine → SIEMENS-S7
- **WHAT**: `_raw` (unprocessed device data)
- **WHICH**: DB1.DW20 (specific PLC register)

## The Pattern

```
umh.v1.<location_path>.<data_contract>[.<virtual_path>].<name>
```

### Breaking It Down

| Part | Example | Purpose |
|------|---------|---------|
| `umh.v1` | Always `umh.v1` | Version prefix |
| `location_path` | `enterprise.site.area.line` | Physical/logical hierarchy |
| `data_contract` | `_raw` or `_pump_v1` | Data structure type |
| `virtual_path` | `motor.diagnostics` | Optional folder organization |
| `name` | `temperature` or `work_order.create` | The data point or action |

## Location Path - WHERE

The location hierarchy organizes your physical and logical structure:

- **Level 0 (Required)**: Enterprise - your company
- **Level 1**: Site - physical location
- **Level 2**: Area - department or zone
- **Level 3**: Line - production line or cell
- **Level 4**: Machine - specific equipment

You can use ISA-95, KKS, or any naming standard. The only rule: level 0 is mandatory.

Example: `umh.v1.acme.chicago.packaging.line1.filler._raw.speed`

## Data Contract - WHAT

The contract defines the data structure:

- **`_raw`**: Unvalidated device data for exploration
- **`_devicemodel_v1`**: Validated device models (e.g., `_pump_v1`, `_cnc_v1`)
- **`_businessmodel_v1`**: Business KPIs and aggregations (e.g., `_maintenance_v1`, `_production_v1`)

Contracts always start with underscore. They're your data's "type system."

## Virtual Path - Organization

Optional segments for grouping related data:

```
umh.v1.acme.plant._pump_v1.motor.diagnostics.vibration
                          └─────┬─────┘
                          Virtual path for organization
```

## Name - WHICH

The specific data point or action:
- **Time-series**: `temperature`, `pressure`, `running`
- **Relational**: `work_order.create`, `batch.complete`

## Next Steps

- [Understand payload formats](payload-formats.md) for message structure
- [Connect devices with bridges](../data-flows/bridges.md)
- [Explore with Topic Browser](topic-browser.md)
