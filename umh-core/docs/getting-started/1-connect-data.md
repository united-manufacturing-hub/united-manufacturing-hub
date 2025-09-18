# Step 2: Connect Your First Data Source

Let's get some data flowing! We'll start with simulated data, then you can connect real devices later.

## Navigate to Data Flows

1. Click **"Data Flows"** in the left menu
2. Go to the "Bridges" tab. Here you find all of your bridges.
3. Click **"Add Bridge"** (that's it - no other options to confuse you!)

![](images/data-flow.png)

## Configure the Bridge - General Tab

**Bridge Name:** `my-first-bridge`
**Instance:** <Your UMH Core instance>
**Location:** Pre-filled with your instance location. You can add more detail later, but for now leave it as is. In the screenshot, we're defining a bridge for `enterprise.siteA`.
**Connection:** Enter the IP/hostname and port of the device you want to connect to. Since we don't have a real PLC for this tutorial, we'll use `localhost` as IP and `8080` as port.

![Screenshot: Bridge creation form with these fields filled](images/bridge-general.png)

Click **"Save and Deploy"**. A popup will show the deployment progress and any errors or warnings. If everything goes well, you'll be automatically redirected.

![](images/bridge-general-deploy.png)

Back on the "General" tab, you can verify the connection was successful. Here we see a latency of "0 ms" since we're connecting to localhost.

![](images/bridge-general-latency.png)

If connecting to a real PLC fails, the latency indicator will turn orange:

![](images/bridge-general-latency-bad.png)

The status "Starting_failed_dfc_missing" means we haven't configured a data flow yet - we've only tested the connection. Let's actually get data flowing by configuring the "Read" tab.

![](images/bridge-read-header.png)

**Protocol:** Select the protocol to read from. Choose "Generate" to simulate data without a real PLC.
**Data Type:** Select "Time Series" (the standard for PLC tags).
**Monitoring:** Shows the bridge state (currently "Starting_failed_dfc_missing") and throughput (currently zero).

![](images/bridge-read-input.png)

**Input:** Since we selected "Generate", we can create test messages. With real protocols like "Modbus" or "Siemens S7", you'd see protocol-specific settings here. For now, use the defaults: generate `hello world` every `1s`.

![](images/bridge-read-processing.png)

**Processing:** The "Tag Processor" appears because we selected "Time Series" data type.

Three required fields:
- **location_path:** Where the data goes (auto-filled from bridge location)
- **data_contract:** Leave as `_raw` for now (no validation rules)
- **tag_name:** Name your data point - we'll use `my_data`

The **Always** section uses JavaScript to process messages. We're not modifying anything for now, just passing the data through.

💡 **Tip:** You can modify data here later (e.g., unit conversions, renaming). If you don't know JavaScript, any LLM (ChatGPT, Claude) can help write the code.

![](images/bridge-read-output.png)

The Output section is auto-generated - it sends data to your Unified Namespace.

Click **"Save & Deploy"**.

![](images/bridge-read-deployed.png)

## 🎉 Success!

You should now see:
- **Status:** Active ✅
- **Throughput:** ~1 msg/sec

Your data is flowing!

## Step 3: View Your Data in the Topic Browser

![](images/topic-browser-my_data.png)

Click **"Topic Browser"** in the left menu. This shows all data in your Unified Namespace.

You'll see your data organized as:
- `enterprise` → `siteA` → `_raw` → `my_data`

Exactly as we configured it! Click on `my_data` to see details.

**Topic Details:** Shows your bridge configuration and data location
**Last Message:** The most recent `hello world` message with timestamp
**History:** A table showing recent messages (would be a chart for numeric data)
**Metadata:** Additional information about the data source (we'll use this later)

## Understanding What You Built

You just created a complete data pipeline:

```
Bridge → Processing → Unified Namespace → Topic Browser
```

**Key Concepts:**
- **Bridge:** The ONLY way data enters UMH (ensures quality and monitoring)
- **Location Path:** Automatically organizes your data (`enterprise.siteA`)
- **Data Contract:** Currently `_raw` (no validation rules)
- **Tag Name:** Your measurement name (`my_data`)

## What's Next?

**You have data flowing!** This is already production-ready for many use cases.

**Want to organize better?** → [Continue to Step 3: Organize Your Data](2-organize-data.md)

**Connect a real device?** Simply change the protocol from "Generate" to:
- **OPC UA** for modern PLCs
- **Modbus** for older equipment
- **MQTT Subscribe** for existing MQTT devices
- [See all 50+ supported protocols →](https://docs.umh.app/benthos-umh/input)

## Concepts Learned

- **Bridge** - The only way data enters UMH Core
- **Data Flows** - Configuration area for bridges
- **Protocol** - How to connect (Generate, OPC UA, Modbus, S7)
- **Connection settings** - IP/hostname and port configuration
- **Latency indicator** - Connection health monitoring
- **Tag Processor** - JavaScript-based data transformation
- **location_path** - Data organization hierarchy
- **data_contract** - Data validation rules (_raw = no validation)
- **tag_name** - Individual measurement identifier
- **Unified Namespace** - Central data repository
- **Topic Browser** - UI for viewing all data
- **Throughput** - Messages per second metric

---

**Pro tip:** Everything you just configured in the UI is stored as YAML. As you get comfortable, you can copy and modify these configurations for faster setup of similar devices.
