# Payload Formats

UMH-Core recognises **three** payload formats. Pick the one that matches your sensor / message **before** you build a bridge or stream-processor.

| Type                   | Typical content                                                           | When to use it                                                                                                                                                                                                   | Producer / Processor     | Sink (example)    |
| ---------------------- | ------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------ | ----------------- |
| **Time-series / Tags** | One numeric/boolean value + timestamp (sensor readings, counters, states) | Data coming from PLCs or sensors                                                                                                                                                                                 | Bridge + `tag_processor` | TimescaleDB       |
| **Relational / JSON**  | One self-contained business record (order, recipe, batch header)          | <p>Data coming from higher-level systems, such as Orders, alarms, set-points, Batch reports, recipes<br><br>OR<br><br>time-series data that belongs together, e.g., that has been merged (see further below)</p> | Bridge + `nodered_js`    | PostgreSQL / REST |
| **Binary Blob**        | File pointer or binary payload (images, PDFs, CNC files)                  | For everything else                                                                                                                                                                                              | Bridge + `nodered_js`    | S3 Bucket         |

### Why you **do not** bundle time-series points into one JSON object

#### The tempting shortcut (UMH Classic)

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

| Hidden problem                     | Why it hurts in real projects                                                                                                                                                            |
| ---------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **No single, unique identifier**   | The true tag is _topic + JSON-key_ (`…weather.temperature`). Every dashboard, rule engine, historian and schema registry must now learn a _two-dimensional_ address space.               |
| **Edge-case combinatorics**        | _Missing key? late arriving field? partial PLC failure?_  Each choice (ignore / cache / null) multiplies test cases and bugs.                                                            |
| **Unintuitive logic for OT users** | <p>They just want to write<br><code>if …weather.temperature > 30 &#x26;&#x26; …isCloudy == true</code><br>not manage local caches or delta-messages.</p>                                 |
| **Merge window guessing**          | Two PLC variables that change _almost_ at the same time still arrive as separate OPC UA notifications. You must decide _how long_ to wait before you believe you have a “complete” JSON. |
| **Clock skew & racing updates**    | Reading 100 tags into one JSON can take milliseconds; by the time the last field is read the first may already have changed—yet a single timestamp suggests they belong together.        |
| **Topic-vs-payload bikeshedding**  | Should a nested struct live under `weather.*` or inside the payload? Different consumers want different splits → endless disagreements.                                                  |

_Result:_ every integrator ends up writing brittle glue scripts and the Tag-Browser (which once parsed the payload to build a “full-tag-name”) turns into a complexity monster.

#### UMH-Core’s guiding rule

> **“One tag, one message, one topic.”**

```
umh.v1.acme._historian.weather.temperature   { "value": 23.4, "timestamp_ms": … }
umh.v1.acme._historian.weather.humidity      { "value": 42.1, "timestamp_ms": … }
```

_What changes?_ – You collapse the **three dimensions**`topic × json-key × payload-format` → **one dimension** `topic`.

The _address_ of a datapoint is now self-contained and stable; the payload is always the single scalar that belongs to that address.

**Benefits:**

* **Intuitive expressions** – both OT & IT write rules against plain topics:`if umh.v1.acme._historian.weather.temperature > 30 …`
* **Zero merge code** – every value is complete at publish time; no caching.
* **Schema enforcement moves to the topic** – data contract can simply specify time-series payload; no further schema needed.
* **Unlimited fan-out** – a client interested **only** in humidity subscribes once; it is not spammed by temperature updates.

#### When you _do_ want a bundled JSON

Quality stations or batch reports often require a final “document” after all samples are taken. Create a **Stream-Processor**, define the merge window & payload, and register an explicit **Data Contract** (e.g. `BatchReport`). All assumptions become version-controlled and auditable.

