# Type Definitions

> This is a reference document. Use it to look up the types that a data model field accepts and the errors the validator may raise.

A field's value type is declared at one of two levels. A data model field references a type with `_payloadshape` (a named payload shape) or `_refModel` (another data model). Inside a payload shape, each field declares a scalar directly with `_type`.

## Field Types (`_type`)

A [payload shape](payload-shapes.md) field's `_type` is one of:

| `_type`   | Meaning                   | Example value |
|-----------|---------------------------|---------------|
| `string`  | Text                      | `"running"`   |
| `number`  | Decimal / floating point  | `42.5`        |
| `integer` | Whole number, no fraction | `42`          |
| `boolean` | True / false              | `true`        |

`number` and `integer` are distinct: `number` accepts a fraction, `integer` does not. Any other value is rejected: `unsupported UMH type: <type>`.

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
                version: v1
          metadata:
            _refModel:
              name: device-info
              version: v1
```

Multiple versions of one model:

```yaml
dataModels:
  - name: sensor-data
    version:
      v1:
        structure:
          value:
            _payloadshape: timeseries-number
      v2:
        structure:
          value:
            _payloadshape: timeseries-number
          status:
            _payloadshape: timeseries-string
```

Define a custom [payload shape](payload-shapes.md) (top-level `payloadShapes:`), then reference it:

```yaml
payloadShapes:
  work-order:
    description: Work order record
    fields:
      orderId:
        _type: string
      quantity:
        _type: integer
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
| `unsupported UMH type: <type>` | A field `_type` is not `string`/`number`/`integer`/`boolean` | Use one of the four allowed types |
| `field cannot have both _payloadshape and _refModel` | A leaf field sets both keys | Keep one |
| `leaf nodes must contain _payloadshape, _relational, or _refModel` | A leaf field has no value-type key | Add `_payloadshape` or `_refModel`, or give the field subfields |
| `non-leaf nodes (folders) cannot have _payloadshape` | A field with subfields also sets `_payloadshape` | Remove `_payloadshape` from the folder |

## Related

- [Data Modeling](README.md) - Concepts and the component chain
- [Payload Shapes](payload-shapes.md) - Built-in and custom shapes
- [Data Models](data-models.md) - Structure, `_refModel`, versions
- [Data Contracts](data-contracts.md) - Enforcement at ingress
- [Payload Formats](../unified-namespace/payload-formats.md) - UNS payload structure
