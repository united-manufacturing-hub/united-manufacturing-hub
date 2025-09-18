# Step 4: Validate Your Data

> **Prerequisite:** You should have multiple tags flowing from [Step 3](2-organize-data.md). If not, complete that first!

## The Problem

Right now, ALL your data goes into `_raw` - no validation:
- DB1.DW20 could suddenly send "hello" instead of a number
- Critical vibration data might arrive with wrong units
- Typos in tag names create duplicate data streams

**How do you ensure data quality?** Data Models!

## Part 1: Create Your First Data Model

Let's create a model for a CNC machine's vibration data.

### Add a Data Model

1. Go to **Data Models** → **Add Data Model**
2. **Instance:** Select your instance
3. **Name:** `cnc`
4. **Description:** "CNC machine vibration monitoring"

### Define the Structure

In the **Data Model Structure** section, add this YAML:

```yaml
vibration:
  x-axis:
    _payloadshape: timeseries-number
  y-axis:
    _payloadshape: timeseries-number
```

**What this means:**
- Your CNC model has a `vibration` folder
- Inside are two measurements: `x-axis` and `y-axis`
- Both must be numbers (enforced automatically!)

![Data Model Creation](images/3-data-model-create.png)

Click **Save & Deploy**.

![Data Model List](images/3-data-model-list.png)

💡 **Behind the scenes:** The system automatically creates a data contract called `_cnc_v1`. This contract will enforce your model's structure. [Learn more about data contracts →](../usage/data-modeling/data-contracts.md)

## Part 2: Use Your Model in a Bridge

### Find Your Contract Name

1. Go to **Contracts** tab
2. Find `_cnc_v1` - this was auto-generated from your model
3. Note the structure it expects

![Contract View](images/3-contract-view.png)

### Update Your Bridge

Go back to your S7 bridge from Step 3. In the **Always** section, change:

```javascript
// OLD: Everything goes to _raw
msg.meta.data_contract = "_raw";
```

To:

```javascript
// NEW: Everything goes to our validated model
msg.meta.data_contract = "_cnc_v1";
```

Click **Save & Deploy**.

## Part 3: Experience Validation (It Will Fail!)

![Deployment Failed](images/3-deployment-failed.png)

**Your deployment fails!** Look at the error:

```
schema validation failed for message with topic 'umh.v1.enterprise.sksk._cnc_v1.DB1.DW20':
Valid virtual_paths are: [vibration.x-axis, vibration.y-axis].
Your virtual_path is: DB1.DW20
```

**The lesson:** Data models ENFORCE structure. Your S7 address `DB1.DW20` doesn't match the expected paths `vibration.x-axis` or `vibration.y-axis`.

**Two new concepts here:**
1. Deployments can fail not only if the connection is bad (learned in Step 2)
2. But also if the bridge throws validation errors

## Part 4: Fix with Smart Routing

Instead of forcing ALL data into the model, let's be selective. Change your **Always** section back:

```javascript
// Most data stays unvalidated
msg.meta.data_contract = "_raw";
msg.meta.tag_name = msg.meta.s7_address;
msg.payload = msg.payload;
return msg;
```

Now update your condition for DB1.DW20:

```javascript
// Special handling for DB1.DW20: Route to validated model
msg.payload = parseFloat(msg.payload) * 1.0;
msg.meta.data_contract = "_cnc_v1";  // Use validated model
msg.meta.virtual_path = "vibration";  // Required: matches model structure
msg.meta.tag_name = "x-axis";         // Required: matches model field
msg.meta.unit = "raw";
return msg;
```

**What this does:**
- DB1.DW20 → Validated as `vibration.x-axis` in CNC model
- All other tags → Continue to `_raw` (no validation)

Click **Save & Deploy**. Now it succeeds!

## Part 5: Success! View Your Validated Data

![Topic Browser with Validated Data](images/3-topic-browser-validated.png)

In **Topic Browser**, you now see:
```
enterprise.sksk._cnc_v1.vibration.x-axis    [12345]  ✓ Validated
enterprise.sksk._raw.DB1.S30.10             ["text"]  (Unvalidated)
enterprise.sksk._raw.DB3.I270               [789]     (Unvalidated)
```

**The magic:**
- The CNC model GUARANTEES `x-axis` is always a number
- If someone sends text, it's rejected at the bridge
- Other data flows normally through `_raw`

## Concepts Learned

Building on previous guides, you now understand:

- **Data Models** - Reusable templates defining data structure
- **Payload Shapes** - Type validation (timeseries-number, etc.)
- **Schema Validation** - Automatic enforcement at the bridge
- **Auto-generated Contracts** - Models create contracts like `_cnc_v1`
- **Validation Errors** - Deployments fail if data doesn't match
- **Selective Validation** - Route specific data to validated models

## What's Next?

You now have a complete data pipeline with:
- ✅ Automatic tag discovery (Step 3)
- ✅ Smart organization (Step 3)
- ✅ Data validation (Step 4)

**Ready for production?** Check out:
- [Production Guide](../production/README.md) - Sizing, security, monitoring
- [Data Modeling Deep Dive](../usage/data-modeling/README.md) - Advanced models
- [Stream Processors](../usage/data-modeling/stream-processors.md) - Transform Silver → Gold

---

**Congratulations!** You've mastered the fundamentals of UMH Core. Your data is now organized, validated, and production-ready. 🎉
