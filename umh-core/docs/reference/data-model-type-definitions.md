# Data Model Type Definitions

> This is a reference document. Use it to look up the types that a data model field accepts and the errors the validator may raise.

A field's value type is declared at one of two levels. A data model field references a type with `_payloadshape` (a named payload shape) or `_refModel` (another data model). Inside a payload shape, each field declares a scalar directly with `_type`.

## Field Types (`_type`)

A [payload shape](payload-shapes.md) field's `_type` is one of:

| `_type`   | Meaning                             | Example value |
|-----------|-------------------------------------|---------------|
| `string`  | Text                                | `"running"`   |
| `number`  | Any numeric value, decimal or whole | `42.5`, `42`  |
| `boolean` | True / false                        | `true`        |

Any other value is rejected: `unsupported UMH type: <type>`.

## Timeseries payload shapes are inherently typed

When defining the `_payloadshape` to be timeseries data, the name of the payload shape already states the type. As such, no additional type definition is needed.

| Payload shape        | `value` type | Use for                         |
|----------------------|--------------|---------------------------------|
| `timeseries-number`  | `number`     | Numeric readings (temperature…) |
| `timeseries-string`  | `string`     | Text status values              |
| `timeseries-boolean` | `boolean`    | On/off, true/false state        |

Each built-in timeseries shape also carries `timestamp_ms` (a `number`) next to `value`. Both fields come from the built-in shape definition, so you never declare `timestamp_ms` yourself. See [Built-in Shapes](payload-shapes.md#built-in-shapes) for the full payload structure.

A `_payloadshape` reference must name one of the three built-ins above or a shape defined under `payloadShapes:`. Any other name fails validation with `referenced payload shape '<name>' does not exist`.

```yaml
temperature:
  _payloadshape: timeseries-number
status:
  _payloadshape: timeseries-string
```

## Version Keys

A data model version key is `v<major>` or `v<major>_<minor>`. `v1` and `v1_0` are the same version: a bare `vN` always means major N, minor 0. Major numbering starts at 1 (major `0` is invalid), and neither part takes a leading zero, except that the minor alone may be the literal `0`.

Creating a new version appends the next minor of the model's highest major. A model at `v1` (that is, `v1_0`) gains `v1_1`. A model that already has both `v1` and `v2` gains `v2_1`, not `v1_2` or `v3_0`. Bumping the major is not supported yet, so a genuinely breaking change (see below) needs a new data model, not a new major version.

### A new version may only add tags

A new version can add a tag but cannot remove one, rename one, or change its payload shape. Renaming a tag is seen as removing the old name and adding a new one, so it is refused for the same reason a removal is. The check compares the new version's tags against the highest existing minor of the same major and refuses the write, before anything is saved, if any tag was removed or changed shape. The error names every offending tag.

A removed tag:

```
cannot add version v1_1 to data model "pump": 1 breaking change

  rpm  removed (was timeseries-number)

A new minor version may only add tags. Changing or removing an existing tag
requires a new major version, which is not supported yet.
```

A tag whose payload shape changed:

```
cannot add version v1_1 to data model "pump": 1 breaking change

  temperature  payload shape changed: timeseries-number -> timeseries-string

A new minor version may only add tags. Changing or removing an existing tag
requires a new major version, which is not supported yet.
```

Do not work around a refusal by deleting the model and recreating it with the change already made. The Historian writes each tag to its own database column, and deleting a data model does not delete that column's data — recreating the model from scratch does not give you a clean slate, it silently orphans the history already written under the old tag.

### Contract naming

Each version gets its own data contract. The name is always `_<model>_<versionKey>`, with the version key spelled in full, including the `_0` of a first minor: a model named `pump` at `v1_0` gets contract `_pump_v1_0`; at `v1_1`, `_pump_v1_1`. A bridge or stream processor must reference the specific contract for the version whose tags it needs — the contract for an earlier version keeps validating that version's tags and does not gain a later version's additions automatically.

## Examples

Data model with several timeseries fields:

```yaml
dataModels:
  - name: temperature
    version:
      v1:
        structure:
          temperature:
            _payloadshape: timeseries-number
          unit:
            _payloadshape: timeseries-string
```

Nested folders, and a field that references another data model (`_refModel`):

```yaml
dataModels:
  - name: complex-model
    version:
      v1:
        structure:
          sensor: # folder here
            temp_reading:
              _payloadshape: timeseries-number
            temp_unit:
              _refModel:
                name: temperature
                version: v1_0
          metadata:
            _refModel:
              name: device-info
              version: v1_0
```

Adding a version to a model, `status` added in `v1_1` alongside everything `v1_0` already had (see [Version Keys](#version-keys)):

```yaml
dataModels:
  - name: sensor-data
    version:
      v1_0:
        structure:
          value:
            _payloadshape: timeseries-number
      v1_1:
        structure:
          value:
            _payloadshape: timeseries-number
          status:
            _payloadshape: timeseries-string
```

This produces contract `_sensor-data_v1_1` alongside the existing `_sensor-data_v1_0`.

Define a custom [payload shape](payload-shapes.md) (top-level `payloadShapes:`), then reference it:

```yaml
payloadShapes:
  work-order:
    description: Work order record
    fields:
      orderId:
        _type: string
      quantity:
        _type: number
      price:
        _type: number
      active:
        _type: boolean

dataModels:
  - name: orders
    version:
      v1:
        structure:
          order:
            _payloadshape: work-order
```

Rules a data model field must follow:

- A leaf field references its type with `_payloadshape` or `_refModel`, never both.
- A folder (a field with subfields) has neither `_payloadshape` nor `_refModel`.

## Validation Errors

| Error message | Cause | Fix |
|---------------|-------|-----|
| `referenced payload shape '<name>' does not exist` | `_payloadshape` names a shape that is not built-in and not defined | Use a built-in, or define the shape first |
| `unsupported UMH type: <type>` | A field `_type` is not a supported type | Use `string`, `number`, or `boolean` |
| `field cannot have both _payloadshape and _refModel` | A leaf field sets both keys | Keep one |
| `leaf nodes must contain _payloadshape, _relational, or _refModel` | A leaf field has no value-type key | Add `_payloadshape` or `_refModel`, or give the field subfields |
| `non-leaf nodes (folders) cannot have _payloadshape` | A field with subfields also sets `_payloadshape` | Remove `_payloadshape` from the folder |

## Related

- [Data Modeling](README.md) - Concepts and the component chain
- [Payload Shapes](payload-shapes.md) - Built-in and custom shapes
- [Data Models](data-models.md) - Structure, `_refModel`, versions
- [Data Contracts](data-contracts.md) - Enforcement at ingress
- [Payload Formats](../unified-namespace/payload-formats.md) - UNS payload structure

> ### Note: the `integer` type
> The YAML validator also accepts `_type: integer` inside a custom payload shape defined in an instance's config file.
> However, there is no built-in `timeseries-integer` shape and the data-model editor does not offer it, so most models never need it.
> Use `number` for numeric values. `integer` is only relevant when a custom payload shape must reject fractional values (for example a discrete count or an ID).
