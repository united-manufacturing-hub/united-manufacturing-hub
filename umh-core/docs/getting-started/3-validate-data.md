# Step 3: Validate Your Data

> **Prerequisite:** You should be comfortable organizing data from [Step 2](2-organize-data.md). Your data should be well-organized but still using `_raw`.

## When You Need Validation

Your `_raw` data works great... until it doesn't:

### Real Problems That Happen:

**Problem 1: Wrong Data Types**
```
Temperature: 22.5     ✅ Expected
Temperature: "ERROR"  ❌ Dashboard crashes
```

**Problem 2: Missing Fields**
```
Normal:   { "pressure": 4.2, "temperature": 80 }     ✅
Suddenly: { "pressure": 4.2 }                        ❌ Where's temperature?
```

**Problem 3: Inconsistent Units**
```
Pump-01: Temperature in Celsius
Pump-02: Temperature in Fahrenheit  
Dashboard: 🔥 Shows pump-02 at 80°C when it's really 80°F
```

## Your First Data Model

Let's create a model for a pump that ensures data quality:

### 1. Go to Data Models (New Section!)

You'll notice a new menu item appears when you need it:

1. Click **"Data Models"** in the left menu
2. Click **"Create Model"**

![Screenshot: Data Models page, empty state]

### 2. Define What a Pump Should Have

**Model Name:** `pump`

**Add Fields:**
Click "Add Field" for each measurement:

| Field Name | Type | Required |
|------------|------|----------|
| pressure | Number | Yes |
| temperature | Number | Yes |
| flow_rate | Number | Yes |
| running | Number (0 or 1) | Yes |

![Screenshot: Model builder with these fields added]

### 3. Click Create

Behind the scenes, the system creates:
- Your model (the template)
- A contract called `_pump_v1` (the enforcer)

**You don't need to know this yet!** Just know your pump model is ready.

## Use Your Model

### 1. Create a New Bridge with Your Model

Go to **Data Flows** → **Add Bridge**

Fill it in as before, but notice something new:

**Data Contract:** Instead of `_raw`, select `_pump_v1`

![Screenshot: Dropdown showing _raw and _pump_v1 options]

### 2. Map Your Data to Model Fields

The bridge now shows your model's fields:

```
Your PLC Tag        →  Model Field
PT101               →  pressure
TT101               →  temperature  
FT101               →  flow_rate
M101.Running        →  running
```

![Screenshot: Mapping interface with model fields]

### 3. Try to Break It (It Won't Let You!)

Click **"Test Connection"**

If your data doesn't match:
```
❌ Error: Expected field 'pressure', got 'presure' (typo)
❌ Error: Field 'temperature' must be a number, got "OFFLINE"
❌ Error: Missing required field 'flow_rate'
```

The bridge goes to **"Degraded"** state and shows you exactly what's wrong!

![Screenshot: Bridge in degraded state with clear error message]

### 4. Fix the Mapping

Correct your mappings until Test Connection shows:
```
✅ All fields validated
✅ Data matches pump model
```

Now click **"Create Bridge"**

## See Your Validated Data

In Topic Browser, your pump data now appears at:
```
umh.v1.plant-1.packaging.line-5.pump-01._pump_v1.pressure
umh.v1.plant-1.packaging.line-5.pump-01._pump_v1.temperature
umh.v1.plant-1.packaging.line-5.pump-01._pump_v1.flow_rate
umh.v1.plant-1.packaging.line-5.pump-01._pump_v1.running
```

Notice `_pump_v1` instead of `_raw` - this means validated data!

### What's Different?

| `_raw` | `_pump_v1` |
|--------|------------|
| Accepts anything | Only accepts pump data |
| No validation | Must have all 4 fields |
| Hope it's right | Guaranteed structure |
| Consumers must handle errors | Consumers can trust the data |

## The Protection in Action

### Try Sending Bad Data:

If your PLC sends corrupted data:
1. Bridge detects it doesn't match the model
2. Bridge goes to "Degraded" state
3. Bad data is BLOCKED from entering
4. You get an alert in the console
5. Good data resumes when PLC is fixed

**Your downstream systems are protected!**

## Create Models for Other Equipment

### Quick Exercise:

Create models for:

**Temperature Sensor** (Simple)
- temperature: Number

**Motor** (Medium)
- current: Number
- voltage: Number  
- rpm: Number
- running: Number

**CNC Machine** (Complex)
- spindle_rpm: Number
- spindle_load: Number
- x_position: Number
- y_position: Number
- z_position: Number
- program_name: Text
- status: Text

Each becomes a contract (`_temperature_v1`, `_motor_v1`, `_cnc_v1`) that bridges can use!

## Models Work Everywhere

The best part: **One model, unlimited locations**

Create the `pump` model once, use it for:
- `plant-1.packaging.line-5.pump-01._pump_v1`
- `plant-1.packaging.line-5.pump-02._pump_v1`
- `plant-2.assembly.line-1.pump-33._pump_v1`
- `plant-7.warehouse.zone-C.pump-99._pump_v1`

All follow the SAME structure!

## What You've Learned

✅ **Models define structure** - What fields and types are required
✅ **Bridges enforce models** - Bad data gets blocked
✅ **One model, many devices** - Reuse across your entire enterprise
✅ **Protection built-in** - Downstream systems never see bad data

## You're Done with Basics!

You now know the THREE core concepts:

1. **Bridges** - How data gets in
2. **Organization** - Location paths (folders)
3. **Models** - Structure and validation

This covers 90% of use cases!

## Optional: Advanced Topics

Most users stop here. But if you need more:

**Different data formats?**
→ [Custom Payload Shapes](../usage/modeling-data/advanced/payload-shapes.md) (Advanced!)

**Transform existing data?**
→ [Stream Processors](../usage/modeling-data/advanced/stream-processors.md) (Very Advanced!)

**Complex business logic?**
→ [Gold-Level Data](../usage/modeling-data/advanced/business-models.md) (Expert!)

## Start Using Your Data

**Ready to build dashboards?**
→ [Consuming Data](../usage/consuming-data/README.md)

**Connect more devices?**
→ [Bridge Examples](../usage/producing-data/common-patterns.md)

**Production deployment?**
→ [Production Guide](../production/README.md)

---

## Remember

🎉 **You've completed the essential learning path!**

Everything else is optional optimization. Your data is:
- ✅ Flowing (bridges)
- ✅ Organized (locations)  
- ✅ Validated (models)

That's production-ready!