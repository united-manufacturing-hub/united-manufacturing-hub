# Stream Processors

> **Advanced Topic**: You probably don't need this yet! Stream processors are for combining multiple devices into business KPIs. If you're just organizing device data, stick with [bridges](../data-flows/bridges.md) and [data models](data-models.md).
>
> **Current Limitation**: Time-series output only. Complex business records coming later.

## Do You Need This?

```
Are you combining data from multiple devices?
├─ No → Use bridges (you already know this!)
└─ Yes → Keep reading about stream processors
```

## Real-World Examples

### What Bridges Handle (You Know This)
- **Rename tags**: `DB1.DW20` → `vibration.x-axis`
- **Convert units**: Fahrenheit → Celsius in Tag Processor
- **Organize data**: Using virtual_path for folders
- **Validate structure**: Using data contracts

### What Stream Processors Handle (New!)
- **Combine 5 CNCs**: Calculate total production across line
- **Maintenance alerts**: Monitor runtime hours from all pumps
- **Quality metrics**: Aggregate defects from multiple stations
- **Energy monitoring**: Sum power usage across factory

## The Big Picture

Remember your CNC from Steps 2-4? That's one machine. But your factory has 50 CNCs.

**Without Stream Processors:**
```
CNC-01._cnc_v1.vibration.x-axis [12.5]
CNC-02._cnc_v1.vibration.x-axis [11.8]
CNC-03._cnc_v1.vibration.x-axis [13.2]
...
CNC-50._cnc_v1.vibration.x-axis [12.1]
```
50 separate data streams!

**With Stream Processors:**
```
Line-A._production_v1.total-vibration [625.3]  // Sum of all
Line-A._production_v1.avg-vibration   [12.5]   // Average
Line-A._production_v1.machines-running [47]    // Count
```
One business metric!

## How It Works: Combining CNCs

Let's aggregate vibration data from your CNC line:

```yaml
# Step 1: Define what to aggregate
templates:
  streamProcessors:
    cnc_line_aggregator:
      model:
        name: production-metrics
        version: v1
      sources:  # Subscribe to multiple CNCs
        cnc1_vib: "enterprise.germany.cnc-01._cnc_v1.vibration.x-axis"
        cnc2_vib: "enterprise.germany.cnc-02._cnc_v1.vibration.x-axis"
        cnc3_vib: "enterprise.germany.cnc-03._cnc_v1.vibration.x-axis"
      mapping:  # Create business metrics
        total-vibration: "cnc1_vib + cnc2_vib + cnc3_vib"
        avg-vibration: "(cnc1_vib + cnc2_vib + cnc3_vib) / 3"
        alert-level: "avg-vibration > 15 ? 'HIGH' : 'NORMAL'"

# Step 2: Deploy the aggregator
streamprocessors:
  - name: line_a_metrics
    _templateRef: "cnc_line_aggregator"
    location:
      0: enterprise
      1: germany
      2: line-a
```

**Result:** One topic with aggregated metrics instead of 50 individual streams!

## Key Difference from Bridges

### Bridges (What You Know)
```javascript
// One input, one output
msg.meta.s7_address  // From ONE PLC
msg.meta.tag_name = "x-axis";  // To ONE topic
```

### Stream Processors (New!)
```yaml
sources:  # MULTIPLE inputs
  cnc1: "topic1"
  cnc2: "topic2"
  cnc3: "topic3"
mapping:  # Combine into NEW metrics
  total: "cnc1 + cnc2 + cnc3"
```

## Simple Example: Pump Maintenance

Your factory has 10 pumps. You want to track which need maintenance.

### The Problem
```
Pump-01._pump_v1.runtime-hours [8750]
Pump-02._pump_v1.runtime-hours [9200]
Pump-03._pump_v1.runtime-hours [7500]
...
Pump-10._pump_v1.runtime-hours [9950]
```

Which pumps exceed 9000 hours? You'd need to check all 10!

### The Solution with Stream Processor
```yaml
templates:
  streamProcessors:
    maintenance_monitor:
      model:
        name: maintenance
        version: v1
      sources:
        pump1: "enterprise.factory._pump_v1.pump-01.runtime-hours"
        pump2: "enterprise.factory._pump_v1.pump-02.runtime-hours"
        # ... all 10 pumps
      mapping:
        pumps-needing-service: |
          var count = 0;
          if (pump1 > 9000) count++;
          if (pump2 > 9000) count++;
          // ... check all pumps
          return count;
        next-maintenance: |
          return Math.min(pump1, pump2, ...);  // Hours until next service
```

**Result:**
```
enterprise.factory._maintenance_v1.pumps-needing-service [3]
enterprise.factory._maintenance_v1.next-maintenance [50]
```

One glance tells you everything!

## When You're Ready for This

You'll know you need stream processors when:

1. **Dashboard overload**: "I have 50 widgets for 50 machines!"
2. **Manual calculations**: "I sum these values in Excel every morning"
3. **Cross-device logic**: "If 3 pumps exceed threshold, alert!"
4. **Business KPIs**: "Show me total production, not individual machines"

If you're not there yet, stick with bridges and models. They handle 90% of use cases.

## Configuration Deep Dive (If You Need It)

### Template Structure
```yaml
templates:
  streamProcessors:
    your_template:
      model:         # Output structure
        name: production-metrics
        version: v1
      sources:       # Input subscriptions
        alias1: "topic1"
        alias2: "topic2"
      mapping:       # Transform logic
        output_field: "JavaScript expression"
```

### JavaScript in Mapping
```yaml
mapping:
  # Direct pass-through
  value: "source1"
  
  # Math operations
  total: "source1 + source2 + source3"
  average: "(source1 + source2) / 2"
  
  # Conditionals
  status: "value > 100 ? 'HIGH' : 'NORMAL'"
  
  # Complex logic
  efficiency: |
    var produced = source1;
    var planned = source2;
    return (produced / planned) * 100;
```

## The Truth About Stream Processors

### What They're Great For
✅ Aggregating device data into business metrics
✅ Creating summary dashboards
✅ Calculating KPIs across equipment
✅ Reducing data volume for cloud

### What They Can't Do (Yet)
❌ Complex event processing
❌ Relational data output (work orders, batches)
❌ Historical calculations
❌ Machine learning

### Should You Use Them?

Ask yourself:
1. Can I solve this with bridge conditions? → Use bridges
2. Do I need multiple devices combined? → Use stream processor
3. Am I creating business KPIs? → Use stream processor
4. Am I just organizing one device? → Use bridges

## Your Next Step

**Not ready for stream processors?** That's normal! Most users succeed with:
- [Bridges](../data-flows/bridges.md) for device connections
- [Data Models](data-models.md) for organization
- [Data Contracts](data-contracts.md) for validation

**Ready for advanced aggregation?** Start simple:
1. Pick 2-3 similar devices
2. Create a sum or average
3. Deploy and test
4. Expand from there

Remember: You already mastered the hard part (bridges and models). Stream processors just combine what you've already built!
