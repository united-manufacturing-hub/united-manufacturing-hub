# Step 2: Organize Your Data

> **Prerequisite:** You should have data flowing with `_raw` from [Step 1](connect-data.md). If not, go back and get that working first!

## What You'll Learn

Right now your data flows into wherever you put it:
```
umh.v1.plant-1.packaging.line-3.sealer._raw.temperature
```

But what if:
- You want to move that sealer to a different line?
- You want to rename `temp_01` to `temperature_fahrenheit`?
- You want to group related sensors together?

You'll learn to "think in folders" - organizing your data like a file system.

## Understanding Your Data's Address

Open the Topic Browser and look at any data point:

![Screenshot: Topic Browser highlighting the topic path]

The "address" has these parts:

```
umh.v1.plant-1.packaging.line-3.sealer._raw.temperature
       └─────────── where ───────────┘   │        │
                                   format┘        └what
```

You control:
- **WHERE** - The location path (your folder structure)
- **WHAT** - The tag name (what you're measuring)

The system controls:
- **FORMAT** - The data contract (we'll keep using `_raw` for now)

## Organize by Location

Let's say you need to move that sealer to line-5:

### Edit Your Bridge:

1. Go to **Data Flows**
2. Click on your bridge name
3. Click **Edit**
4. Change **Line** from `line-3` to `line-5`
5. Click **Save**

![Screenshot: Editing location in bridge settings]

### See the Change:

In Topic Browser, your data now appears at:
```
umh.v1.plant-1.packaging.line-5.sealer._raw.temperature
                          ↑
                      It moved!
```

**The old location stops updating, the new one starts.** This is like moving a file to a different folder!

## Rename Your Measurements

Maybe your PLC sends cryptic names like `AI_001_PV`. Let's fix that:

### In Your Bridge Settings:

1. Click **Edit** on your bridge
2. Find the **Tag Mappings** section
3. You'll see your original tag names from the device

![Screenshot: Tag mapping interface showing original names]

### Add Friendly Names:

| Device sends | Rename to |
|-------------|-----------|
| AI_001_PV | temperature |
| AI_002_PV | pressure |
| DI_001 | door_open |
| CNT_001 | parts_count |

Click **Save**

### The Result:

Instead of:
```
umh.v1.plant-1.packaging.line-5.sealer._raw.AI_001_PV  ❌ What is this?
```

You now have:
```
umh.v1.plant-1.packaging.line-5.sealer._raw.temperature  ✅ Clear meaning!
```

## Group Related Data

You can add more levels to organize better. Think of it like creating subfolders:

### Standard Levels (Optional):

The location path can have up to 10 levels. Common patterns:

**ISA-95 Style:**
```
enterprise.site.area.line.workcell.device
```

**Functional Style:**
```
region.plant.building.department.process.equipment
```

**Custom Style:** (Yours can be anything!)
```
customer.factory.workshop.machine.subsystem.component
```

### Example: Adding Subsystems

If your sealer has multiple subsystems:

1. Edit the bridge
2. Add level 5: `heater`
3. Map temperature tags there

Now you get:
```
umh.v1.plant-1.packaging.line-5.sealer.heater._raw.temperature
umh.v1.plant-1.packaging.line-5.sealer.heater._raw.setpoint
umh.v1.plant-1.packaging.line-5.sealer.conveyor._raw.speed
umh.v1.plant-1.packaging.line-5.sealer.conveyor._raw.running
```

It's organized like folders:
```
sealer/
├── heater/
│   ├── temperature
│   └── setpoint
└── conveyor/
    ├── speed
    └── running
```

## Think in Folders

The key mental model: **UMH organizes data like a file system**

- Each level is a folder
- Tags are files in those folders
- You can reorganize without losing data
- Consumers can subscribe to entire "folders"

### Example: Subscribe to Everything from Line 5

A dashboard can subscribe to:
```
umh.v1.plant-1.packaging.line-5.+.+._raw.+
```

This gives ALL data from line-5, regardless of which machine or measurement!

## Try It Yourself

### Exercise 1: Plant-Wide View
Create bridges for different areas and see how they organize:
- Packaging line-3
- Assembly line-1
- Warehouse zone-A

### Exercise 2: Consistent Naming
Rename all temperature sensors to follow a pattern:
- `temperature_celsius`
- `temperature_fahrenheit`
- `temperature_setpoint`

### Exercise 3: Hierarchical Organization
Add detail levels for a complex machine:
```
plant-1/machining/cnc-01/spindle/motor/temperature
plant-1/machining/cnc-01/spindle/motor/current
plant-1/machining/cnc-01/axes/x/position
plant-1/machining/cnc-01/axes/y/position
```

## What You've Learned

✅ **Data addresses are like folder paths** - Organize hierarchically
✅ **You control the structure** - Change locations and names anytime
✅ **Think in folders** - Group related data together
✅ **Subscribers can use wildcards** - Get entire "folders" of data

## Still Using _raw - That's OK!

Notice we haven't talked about:
- Data models
- Validation
- Data contracts
- Payload shapes

**You don't need them yet!** Many users run production with just `_raw` and good organization.

## When You Need More

Using `_raw` works great until:

- 🚫 Bad data crashes your dashboard (string instead of number)
- 🚫 Different devices send different formats
- 🚫 You need guaranteed data structure

**Ready to add validation?**
→ [Step 3: Validate Your Data](3-validate-data.md)

**Happy with _raw?**
→ [Start Consuming Data](../usage/consuming-data/README.md)

---

## Quick Tips

💡 **Location changes don't affect history** - Old data stays at old address

💡 **Plan your hierarchy early** - Easier than reorganizing later

💡 **Use consistent patterns** - All pumps → `pump-01`, `pump-02`, etc.

💡 **Keep it simple** - 3-5 levels usually enough
