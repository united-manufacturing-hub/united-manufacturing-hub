# Payload Formats

UMH Core recognizes **two** payload formats. Pick the one that matches your sensor / message **before** you build a bridge or stream-processor.

### TLDR

<table><thead><tr><th>Type</th><th width="260.609375">Typical content (must be a JSON object)</th><th>When to use it</th><th>Producer / Processor</th><th>Sink (example)</th></tr></thead><tbody><tr><td><strong>Time-series / Tags</strong></td><td><strong>Exactly two keys:</strong><br>• <code>timestamp_ms</code> – int / float without fraction<br>• <code>value</code> – scalar number, boolean <strong>or</strong> string<br>Max decoded size ≤ 1024 B</td><td>PLC &#x26; OT sensors streaming individual datapoints</td><td>Bridge + <code>tag_processor</code></td><td>TimescaleDB</td></tr><tr><td><strong>Relational / JSON</strong></td><td>One self-contained business record <strong>or</strong> multi-field snapshot (UMH Classic)<br>(any key set, as long as the root is an object)</td><td>Orders, batch headers, alarms, set-points <strong>or</strong> merged/aggregated time-series</td><td>Bridge + <code>nodered_js</code></td><td>PostgreSQL / REST</td></tr></tbody></table>

Arrays, primitives and raw binaries are rejected by the UNS Output Plugin – wrap them in a JSON object first if you need to transmit such data.

### Exact validation rules

| Property            | Time-series                                                     | Relational                   |
| ------------------- | --------------------------------------------------------------- | ---------------------------- |
| Root type           | JSON object (map)                                               | JSON object (map)            |
| Mandatory keys      | `timestamp_ms`, `value` **only**                                | ≥ 1 arbitrary keys           |
| Forbidden keys      | Any extra key (e.g. `temperature`)                              | —                            |
| `timestamp_ms` type | Integer (signed/unsigned) or float **without** fraction         | _optional_                   |
| `value` type        | <p>Scalar:<br>- number (float/int)<br>- boolean<br>- string</p> | _n/a_                        |
| Illegal values      | `NaN`, `+Inf`, `-Inf`, payload > 1024 B                         | — (size limit still applies) |
| Max payload size    | 1024 bytes after JSON decoding                                  | 1024 bytes                   |
| Examples            | `{"timestamp_ms":1717083000000,"value":23.4}`                   | `{"order_id":42, ...}`       |

### Canonical Examples

**✅ Valid time-series (float)**

```json
{ "timestamp_ms": 1717083000000, "value": 23.4 }
```

**✅ Valid time-series (string)**

```json
{ "timestamp_ms": 1717083000000, "value": "running" }
```

**❌ Invalid time-series, but ✅ valid relational data: extra field**

```json
{ "timestamp_ms": 1717083000000, "temperature": 23.4, "humidity": 42.1 }
```

### Why so strict? (“one tag, one message, one topic”)

In UMH Classic you could publish a whole weather snapshot in one go:

**Topic:**

```
umh.v1.acme._historian.weather
```

**Payload:**

```json
{
  "timestamp_ms": 1717083000000,
  "temperature": 23.4,
  "humidity":    42.1
}
```

#### Where that shortcut explodes

| Hidden problem                     | Why it hurts in real projects                                                                                                                                                                                                                                                    |
| ---------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **No single, unique identifier**   | You need **three dimensions** to access a tag: topic, tag name, and JSON path. Every dashboard, rule engine, historian and schema registry must now learn this complex address space.                                                                                            |
| **Real-world timing issues**       | _What happens when the source gives you a new temperature value, but no humidity value?_ In OPC UA subscriptions, humidity might arrive 100ms later. Do you cache it? How do you know if you're receiving a cached value or if the temperature really stayed constant for hours? |
| **Unintuitive logic for OT users** | <p><strong>As an OT person, you simply want to write:</strong><br><code>if …weather.temperature > 30 &#x26;&#x26; …isCloudy == true</code><br><strong>Not manage local caches or three-dimensional addressing.</strong></p>                                                      |
| **Edge-case combinatorics**        | _Missing key? Late arriving field? Partial PLC failure?_ Each choice (ignore / cache / null) multiplies test cases and bugs.                                                                                                                                                     |
| **Merge window guessing**          | Two PLC variables that change _almost_ at the same time still arrive as separate OPC UA notifications. You must decide _how long_ to wait before you believe you have a "complete" JSON.                                                                                         |
| **Clock skew & racing updates**    | Reading 100 tags into one JSON can take milliseconds; by the time the last field is read the first may already have changed—yet a single timestamp suggests they belong together.                                                                                                |
| **Topic-vs-payload bikeshedding**  | Should a nested struct live under `weather.*` or inside the payload? Different consumers want different splits → endless disagreements.                                                                                                                                          |

_Result:_ every integrator ends up writing brittle glue scripts and the Tag-Browser (which once parsed the payload to build a "full-tag-name") turns into a complexity monster.

#### UMH Core's guiding rule

> **"One tag, one message, one topic."**

```
umh.v1.acme._historian.weather.temperature   { "value": 23.4, "timestamp_ms": … }
umh.v1.acme._historian.weather.humidity      { "value": 42.1, "timestamp_ms": … }
```

_What changes?_ – You collapse the **three dimensions**`topic × json-key × payload-format` → **one dimension** `topic`.

The _address_ of a datapoint is now self-contained and stable; the payload is always the single scalar that belongs to that address.

**Benefits:**

* **Intuitive expressions** – both OT & IT write rules against plain topics:`if umh.v1.acme._historian.weather.temperature > 30 …`
* **Zero merge code** – every value is complete at publish time; no caching.
* **Simplified business discussions** – You only need to discuss the "folder structure" (topic hierarchy) with business stakeholders. No need to explain the difference between tag names, topics, JSON paths, and payload formats.
* **Schema enforcement moves to the topic** – data contract can simply specify time-series payload; no further schema needed.
* **Unlimited fan-out** – a client interested **only** in humidity subscribes once; it is not spammed by temperature updates.

#### When you _do_ want a bundled JSON

Quality stations or batch reports often require a final "document" after all samples are taken. Create a **Stream-Processor**, define the merge window & payload, and register an explicit **Data Contract** (e.g. `BatchReport`). All assumptions become version-controlled and auditable.

## Advanced Examples

### CNC Mill with Multiple Sensor Groups

CNC machines often have multiple sensor groups for different aspects of operation. Here's how to organize related sensors using virtual paths:

**Topic Structure:**

```
umh.v1.acme.plant1.machining.cnc-mill-1234._cnc.axis.x_position
umh.v1.acme.plant1.machining.cnc-mill-1234._cnc.axis.y_position  
umh.v1.acme.plant1.machining.cnc-mill-1234._cnc.axis.z_position
umh.v1.acme.plant1.machining.cnc-mill-1234._cnc.machine_state.status
umh.v1.acme.plant1.machining.cnc-mill-1234._cnc.machine_state.program_name
umh.v1.acme.plant1.machining.cnc-mill-1234._cnc.spindle.rpm
umh.v1.acme.plant1.machining.cnc-mill-1234._cnc.spindle.load_percent
```

**Payloads:**

```json
// Axis positioning data
{
  "value": 125.7,
  "timestamp_ms": 1733904005123
}

// Machine state data  
{
  "value": "running",
  "timestamp_ms": 1733904005123
}

// Spindle data
{
  "value": 3200,
  "timestamp_ms": 1733904005123
}
```

**Virtual Path Benefits:**

* `axis.*` groups all positioning sensors
* `machine_state.*` groups operational status
* `spindle.*` groups spindle-related measurements
* Each group can be consumed independently for specialized analytics

### Packaging Line with Process Data

Packaging equipment often needs to track product flow, quality metrics, and machine health:

**Topic Structure:**

```
umh.v1.foods-corp.chicago.packaging.line-3._packaging.production.units_per_minute
umh.v1.foods-corp.chicago.packaging.line-3._packaging.production.current_product_id
umh.v1.foods-corp.chicago.packaging.line-3._packaging.quality.reject_count
umh.v1.foods-corp.chicago.packaging.line-3._packaging.quality.seal_pressure
umh.v1.foods-corp.chicago.packaging.line-3._packaging.diagnostics.vibration
umh.v1.foods-corp.chicago.packaging.line-3._packaging.diagnostics.temperature
```

**Analytics Use Cases:**

* **Production Dashboard**: Subscribe to `production.*` topics
* **Quality Monitoring**: Subscribe to `quality.*` topics
* **Predictive Maintenance**: Subscribe to `diagnostics.*` topics
* **Overall Equipment Effectiveness**: Combine all virtual paths

### Multi-Sensor Aggregation

Some applications require combining multiple sensors into structured payloads:

**Bridge Configuration:**

```yaml
# Multiple sensors from same device
sources:
  temp_f: "umh.v1.acme.plant1.line4.sensor1._raw.temperature_f"
  humidity: "umh.v1.acme.plant1.line4.sensor1._raw.humidity_pct"
  pressure: "umh.v1.acme.plant1.line4.sensor1._raw.pressure_kpa"

# Stream processor combines into environmental data
mapping:
  environment.temperature_c: "(temp_f - 32) * 5 / 9"
  environment.humidity_percent: "humidity"
  environment.pressure_kpa: "pressure"
```

**Result Topics:**

```
umh.v1.acme.plant1.line4.sensor1._environmental.environment.temperature_c
umh.v1.acme.plant1.line4.sensor1._environmental.environment.humidity_percent  
umh.v1.acme.plant1.line4.sensor1._environmental.environment.pressure_kpa
```
