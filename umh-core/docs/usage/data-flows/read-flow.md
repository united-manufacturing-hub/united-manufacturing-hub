# Read Flow

The read flow is the part of a [bridge](bridges.md) that reads from the external system and publishes to the UNS. To configure it in the Management Console, see [Set up a read flow](set-up-a-read-flow.md).

## Reference

### Configuration

The read flow is the `dataflowcomponent_read` key of a bridge. The full bridge configuration is in [Bridges](bridges.md#configuration).

```yaml
protocolConverter:
  - name: my-plc-bridge
    protocolConverterServiceConfig:
      config:
        dataflowcomponent_read:
          benthos:
            input:
              s7comm:
                addresses:
                  - DB1.DW20
                  - DB3.I270
                tcpDevice: "{{ .IP }}"
                rack: 0
                slot: 1
            pipeline:
              processors:
                - tag_processor:
                    defaults: |
                      msg.meta.location_path = "{{ .location_path }}";
                      msg.meta.data_contract = "_raw";
                      msg.meta.tag_name = msg.meta.s7_address;
                      return msg;
            output:
              uns: {}
```

| Field | Description |
|---|---|
| `protocolConverterServiceConfig.config.dataflowcomponent_read.benthos` | Read flow: the Benthos pipeline that reads from the external system and publishes to the UNS. |

### Metrics

For a bridge named `mixing-station`, the read flow's series carry these labels. [Bridges: Metrics](bridges.md#metrics) explains the labels of the other parts of the bridge.

| Part of the bridge | `component` | `instance` |
|---|---|---|
| Its read flow | `dataflow_component_instance` | `read-protocolconverter-mixing-station` |
| The Benthos process of its read flow | `benthos_instance` | `dataflow-read-protocolconverter-mixing-station` |
