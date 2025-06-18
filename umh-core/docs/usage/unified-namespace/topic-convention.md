# Topic Convention

```
umh.v1.<location_path>.<data_contract>[.<virtual_path>].<tag_name>
```

| Segment            | Filled by `tag_processor` meta field  | Description & rules                                                                                                                                               |
| ------------------ | ------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **umh.v1**         | —                                     | Constant product / schema generation prefix.                                                                                                                      |
| **location\_path** | `msg.meta.location_path`              | Dot-separated ISA-95 hierarchy – **exactly six** levels are expected, but trailing empty levels are omitted:`<enterprise>.<site>.<area>.<line>.<workCell>.<unit>` |
| **data\_contract** | `msg.meta.data_contract`              | Needs to start with an underscore. Logical model that the payload conforms to (e.g. `_historian`, `Pump`, `TemperatureSensor`).                                   |
| **virtual\_path**† | `msg.meta.virtual_path` _(optional)_  | Zero-to-many sub-segments used by explicit contracts to address **sub-models** or **folders** (e.g. `motor.diagnostics`).                                         |
| **tag\_name**      | `msg.meta.tag_name` _or_ auto-derived | Leaf field inside the contract (`temperature`, `power`, `status`).                                                                                                |

† _optional segments – omitted when empty._

With this convention every topic uniquely answers:

* **Where** did the data originate? → `location_path`
* **What** does it represent? → `data_contract` + (`virtual_path`) + `tag_name`
* **Version** of the infrastructure → `umh.v1`
