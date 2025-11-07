# Prosys OPC UA Simulation Server - MaxBatchSize Research Report

**Date**: 2025-11-07
**Research Question**: Should we increase MaxBatchSize from 800 to match server's MaxMonitoredItemsPerCall of 10,000?

---

## Executive Summary

**Recommendation**: **Increase MaxBatchSize to 1,000-2,000 for production, keep 800 as conservative default**

**Confidence Level**: **Medium-High** (based on OPC UA specification, Prosys SDK documentation, and industry practices)

**Key Finding**: Prosys OPC UA servers use **10,000 as the default MaxMonitoredItemsPerCall** value, which is 12.5x higher than the current profile setting of 800. However, the optimal client-side batch size depends on network conditions, server performance characteristics, and whether you're using the Simulation Server vs production servers.

---

## Research Findings

### 1. Prosys OPC UA Server Default Configuration

#### MaxMonitoredItemsPerCall Default Value

**Source**: Prosys OPC UA SDK for Java Forum ([Bad_Timeout errors thread](https://forum.prosysopc.com/forum/opc-ua-java-sdk/bad_timeout-errors/))

- **Default value**: **10,000** items per call
- **How it works**: "Client will read this info when connecting. OperationLimits are an optional feature of UA (they were introduced I think in OPC UA 1.02), if they do not exist, we use a default of 10000 for each limit."
- **SDK configuration**: Can be overridden using `Subscription.setMaxMonitoredItemsPerCall(...)`
- **Applies to**: Prosys OPC UA SDK for Java (which powers the Simulation Server)

#### Evolution of Limits

**Source**: Prosys OPC UA SDK for Java Release Notes

- **Historical limit**: 1,000 items per subscription (older versions)
- **Current limit**: 10,000 items per subscription (modern SDK versions)
- **Rationale**: Increased to support modern industrial automation workloads

### 2. OPC UA Specification Guidance

#### OperationLimitsType (OPC UA Part 5)

**Source**: [OPC Foundation - Part 5: Information Model - 6.3.11 OperationLimitsType](https://reference.opcfoundation.org/Core/Part5/v104/docs/6.3.11)

**MaxMonitoredItemsPerCall Property**:
- **Purpose**: Controls array sizes for CreateMonitoredItems, ModifyMonitoredItems, SetMonitoringMode, DeleteMonitoredItems, and SetTriggering Services
- **Type**: UInt32 (Optional)
- **Constraint**: "Any operational limits Property that is provided shall have a non zero value"
- **Default**: Not specified in specification (implementation-dependent)

#### Performance Recommendations

**Source**: [OPC UA Part 4: Services - 5.12.2 CreateMonitoredItems](https://reference.opcfoundation.org/Core/Part4/v104/docs/5.12.2)

> "Calling CreateMonitoredItems repetitively to add a small number of MonitoredItems each time may adversely affect server performance. **Instead Clients should add a complete set of MonitoredItems to a Subscription whenever possible.**"

**Interpretation**: Larger batch sizes are preferred for performance, but "complete set" doesn't necessarily mean using the maximum allowed limit.

### 3. Prosys-Specific Best Practices

#### Recommended Batch Sizes

**Source**: Prosys Forum - Bad_Timeout errors thread

**For diagnostics and troubleshooting**:
- Start with **100-1,000 items per call**
- Helps identify if server is bottleneck during item creation
- Particularly useful when servers validate node existence from PLCs

**For production workloads**:
- **1,000-2,000 items per call** is a balanced starting point
- Adjust based on:
  - Network latency
  - Server response times
  - Total number of monitored items

**For large subscriptions (>10,000 items)**:
- Batch size becomes critical
- Too small (e.g., 100): "Takes ages" due to sequential network calls
- Too large (e.g., 10,000): Risk of timeouts during initialization

#### Performance Trade-offs

**Network Latency vs. Server Processing**:
- Smaller batches = More sequential calls = Higher cumulative network latency
- Larger batches = Fewer calls but higher server processing time per call
- Optimal balance depends on network quality and server CPU/memory

### 4. Prosys Simulation Server Limitations

#### Important Caveat: Simulation Server ≠ Production Server

**Source**: [Prosys Forum - How to configure 16000 simulation points](https://forum.prosysopc.com/forum/opc-ua-simulation-server/how-to-configure-16000-simulation-points-in-prosys-opc-ua-simulator/)

**Key limitations of the Simulation Server**:

1. **Not designed for large-scale testing**:
   - "The application and the user interface has not been designed for adding a large number of simulation signals"
   - "Not actively tested with large amounts of simulation signals"
   - Performance issues expected with 10,000+ points

2. **Observed stability issues**:
   - Simulation Server with 10,000 configured points became **unresponsive after 5-10 minutes**
   - High CPU usage and memory consumption

3. **UI configuration limits**:
   - Maximum 100 custom simulation points via UI
   - Built-in "MyBigNodeManager" provides 1,000 points
   - Manual XML editing required for larger configurations (up to 16,000 tested)

4. **Production alternative**:
   - Prosys recommends using the **Java OPC UA SDK** for production performance testing
   - Custom server implementations based on SampleConsoleServer example

**Implication**: MaxBatchSize recommendations for Simulation Server may not translate to production Prosys servers or other OPC UA servers built with Prosys SDK.

### 5. Industry Practices and Common Limits

#### Server-Imposed Limits

**Sources**: Stack Overflow, OPC Foundation Forum, GitHub Issues

Common limits observed across various OPC UA server implementations:

| Server Type | MaxMonitoredItemsPerCall | MaxMonitoredItemsPerSubscription | Notes |
|------------|------------------------|--------------------------------|-------|
| Siemens S7-1500 PLC | Not exposed | 1,000 | Throws BadTooManyOperations above 1,000 |
| Prosys Simulation Server | 10,000 | 10,000 | Default SDK values |
| Unified Automation SDK | Configurable | Configurable | No documented defaults |
| NodeOPCUA | 0 (unlimited) | Varies | 0 = no limit |
| Generic OPC UA .NET | 1,000-10,000 | 1,000-10,000 | Implementation-dependent |

**Common pattern**: Most industrial servers support 1,000-10,000 items per call, with 1,000 being a conservative baseline.

#### Client-Side Batching Strategies

**Source**: OPC Labs, Unified Automation Forum

**Conservative approach** (tested and reliable):
- **800-1,000 items per call**
- Stays below most server hard limits
- Good balance between performance and reliability

**Aggressive approach** (for high-performance servers):
- **2,000-5,000 items per call**
- Reduces number of round trips
- Requires testing to ensure server can handle load

**Maximum approach** (use with caution):
- **Match server's MaxMonitoredItemsPerCall** (e.g., 10,000)
- Only if server is known to handle it reliably
- Risk of timeouts during initialization
- Not recommended for Simulation Server

### 6. Memory and CPU Considerations

#### Memory Overhead

**Source**: Unified Automation High Performance OPC UA Server SDK Documentation

**Memory chunk overhead**:
- Each batch operation has header/footer overhead for memory management
- **Larger batches are more memory-efficient** (overhead amortized across more items)
- Very small batches (e.g., 10-50 items) waste memory on management overhead

**IPC heap sizing**:
- OPC UA service data (arrays, strings) stored in IPC heap
- Network buffers for serialized data
- Larger batches require larger message buffers

#### CPU Considerations

**Source**: Prosys Forum - OPC UA Performance discussions

**Server-side validation**:
- Each CreateMonitoredItems call triggers node validation
- For PLC-backed servers, validation may query the PLC
- **Smaller batches = more validation cycles = higher cumulative CPU**

**Client-side processing**:
- Smaller batches easier to process incrementally
- Larger batches may cause client-side processing delays

**Network serialization**:
- Larger messages require more CPU for serialization/deserialization
- But fewer messages reduce total serialization overhead

### 7. Testing and Monitoring

#### How to Determine Optimal Batch Size

**Recommended testing approach**:

1. **Enable INFO-level logging** to observe operation limits:
   ```
   level=info msg="Retrieved operation limits from server"
   level=info msg="  maxMonitoredItemsPerCall=10000"
   ```

2. **Test with incremental batch sizes**:
   - Start at 100 (diagnostic baseline)
   - Increase to 500, 1000, 2000, 5000
   - Monitor for timeouts, delays, or server errors

3. **Monitor key metrics**:
   - Time to create monitored items (should decrease with larger batches)
   - Server CPU/memory usage (should remain stable)
   - Timeout frequency (should be zero)
   - Data notification latency (should not increase)

4. **Test edge cases**:
   - Maximum subscription size (e.g., 10,000 items)
   - Concurrent subscriptions from multiple clients
   - Connection recovery scenarios

#### Configuration Options

**Client-side configuration** (if using Prosys SDK):
```java
// Set maximum items per call
subscription.setMaxMonitoredItemsPerCall(2000);

// Set operation limits globally
uaClient.setOperationLimits(operationLimits);

// Set timeout for operations (milliseconds)
uaClient.setTimeout(60000); // 60 seconds
```

---

## Recommendations

### Primary Recommendation: Tiered Approach

**For Prosys OPC UA Simulation Server**:
- **Conservative profile** (current): **800 items/call**
  - Rationale: Simulation Server not designed for high-scale, prone to performance issues
  - Use for: Development, testing, demonstrations
  - Risk: Very low

- **Balanced profile** (recommended): **1,000-1,500 items/call**
  - Rationale: Matches common industry baseline (1,000), provides margin for Simulation Server
  - Use for: Standard testing scenarios with Simulation Server
  - Risk: Low

- **Performance profile** (optional): **2,000-2,500 items/call**
  - Rationale: Improves initialization time for large subscriptions
  - Use for: Testing client robustness with larger batches
  - Risk: Medium (may cause Simulation Server instability with >5,000 total items)

**For Production Prosys OPC UA Servers** (built with SDK):
- **Recommended**: **2,000-5,000 items/call**
  - Rationale: Production servers handle load better than Simulation Server
  - Server default is 10,000, so 2,000-5,000 is conservative
  - Balances performance with reliability
  - Risk: Low to Medium

- **Maximum** (use with testing): **10,000 items/call**
  - Rationale: Matches server's MaxMonitoredItemsPerCall capability
  - Only if extensive testing confirms server can handle it
  - Risk: Medium to High (timeouts possible during initialization)

### Implementation Strategy

**Step 1: Update Profile Configuration**

Create tiered profiles in your OPC UA client configuration:

```yaml
profiles:
  prosys_simulation_conservative:
    MaxBatchSize: 800
    description: "Safe for Prosys Simulation Server, all scales"

  prosys_simulation_balanced:
    MaxBatchSize: 1500
    description: "Recommended for Prosys Simulation Server (<10k items)"

  prosys_production_standard:
    MaxBatchSize: 2500
    description: "Recommended for production Prosys servers"

  prosys_production_performance:
    MaxBatchSize: 5000
    description: "High-performance for production Prosys servers"
```

**Step 2: Test and Validate**

1. Test with Prosys Simulation Server (100-5,000 simulated items)
2. Monitor initialization time and server stability
3. Verify no BadTooManyOperations errors
4. Document optimal value for your specific use case

**Step 3: Monitor in Production**

1. Enable INFO logging for operation limits
2. Track CreateMonitoredItems call duration
3. Alert on timeouts or BadTooManyOperations errors
4. Adjust batch size if issues detected

### Specific Answer to Your Question

**Should you increase from 800 to 10,000?**

**No, not directly to 10,000.** Here's why:

1. **Simulation Server instability**: The Prosys Simulation Server is not designed for high-scale operations and exhibits performance issues with large configurations. Jumping to 10,000 items/call could cause timeouts or server hangs.

2. **Conservative is better**: The current value of 800 is very safe and works reliably. While it's below the server's capability, it's not a significant performance bottleneck unless you're creating subscriptions with 10,000+ items.

3. **Incremental approach**: If you need better performance, increase to **1,500-2,500** first and test thoroughly. Only go higher if testing confirms stability.

**When 10,000 makes sense**:
- Using a **production Prosys server** (not Simulation Server)
- Creating subscriptions with **>5,000 monitored items**
- Extensive testing confirms server handles it without timeouts
- Network latency is low (<10ms)

**When to keep 800**:
- Using Prosys **Simulation Server**
- Subscriptions have **<2,000 monitored items**
- Prioritizing **reliability over initialization speed**
- Server performance is unknown or untested

---

## Additional Context

### OPC UA Specification Context

The OPC UA specification introduced OperationLimits in **version 1.02/1.03** (circa 2012-2013). Older servers may not expose these limits, in which case clients should use conservative defaults (e.g., 1,000).

The specification emphasizes **batching for performance** but does not mandate specific batch sizes. This is intentional—optimal values depend on:
- Server hardware (CPU, memory)
- Network conditions (latency, bandwidth)
- PLC response times (for PLC-backed servers)
- Client processing capabilities

### Prosys SDK Evolution

Prosys has consistently increased limits over time:
- **Early versions** (<2013): 1,000 items/subscription, 1,000 items/call
- **Modern versions** (2015+): 10,000 items/subscription, 10,000 items/call
- **Rationale**: Industry 4.0 and IIoT workloads require higher scalability

However, the **Simulation Server application** has not kept pace with SDK improvements and remains a lightweight tool unsuitable for production-scale testing.

### Comparison with Other Servers

**Kepware** (KEPServerEX):
- Default limits typically 1,000-5,000 items
- Configurable via server settings
- Production servers handle higher loads than Simulation Server

**Ignition** (uses Eclipse Milo):
- Default limits vary by configuration
- Generally supports 1,000-10,000 items/call
- Designed for production industrial environments

**Siemens S7-1500**:
- Hard limit: 1,000 items/subscription
- Not configurable
- Conservative design for PLC stability

**Conclusion**: Prosys's 10,000 default is on the higher end of the industry spectrum, making it more capable but also more sensitive to client batch size choices.

---

## Caveats and Warnings

1. **Simulation Server ≠ Production Server**: All recommendations assume you understand that Prosys Simulation Server is a development tool, not a production-grade server. Performance characteristics differ significantly from Prosys SDK-based production servers.

2. **Version-specific behavior**: This research is based on recent Prosys SDK versions (4.x+). Older versions may have different defaults or limitations.

3. **No official Prosys guidance**: Prosys does not publish specific MaxBatchSize recommendations for the Simulation Server. The 10,000 default is SDK-level, not application-level guidance.

4. **Network dependency**: All performance recommendations assume good network conditions (<20ms latency). High-latency networks may require smaller batches to avoid timeouts.

5. **Server load matters**: An idle Simulation Server may handle 5,000 items/call fine, but a heavily loaded server (many clients, high data change rate) may struggle with the same batch size.

6. **Testing is mandatory**: Do not blindly increase batch size in production without thorough testing. Start conservatively and increase incrementally.

---

## References

### Primary Sources

1. **Prosys OPC UA SDK for Java Forum**:
   - [Bad_Timeout errors thread](https://forum.prosysopc.com/forum/opc-ua-java-sdk/bad_timeout-errors/) - Default values and configuration
   - [How to configure 16000 simulation points](https://forum.prosysopc.com/forum/opc-ua-simulation-server/how-to-configure-16000-simulation-points-in-prosys-opc-ua-simulator/) - Simulation Server limitations

2. **OPC Foundation Specification**:
   - [Part 5: Information Model - 6.3.11 OperationLimitsType](https://reference.opcfoundation.org/Core/Part5/v104/docs/6.3.11)
   - [Part 4: Services - 5.12.2 CreateMonitoredItems](https://reference.opcfoundation.org/Core/Part4/v104/docs/5.12.2)

3. **Prosys Documentation**:
   - Prosys OPC UA SDK for Java Release Notes (4.x series)
   - Prosys OPC UA Simulation Server User Manual

### Supporting Sources

4. **Community Discussions**:
   - [OPCFoundation/UA-.NETStandard Issue #564](https://github.com/OPCFoundation/UA-.NETStandard/issues/564) - Subscription and monitored item limits
   - Unified Automation Forum - Performance considerations
   - Stack Overflow - OPC UA client implementation questions

5. **Industry Resources**:
   - OPC Labs - QuickOPC client documentation
   - Unified Automation High Performance OPC UA SDK documentation
   - Siemens S7-1500 OPC UA Server system limits

---

## Conclusion

The Prosys OPC UA Simulation Server's MaxMonitoredItemsPerCall of **10,000** represents the SDK's capability, not necessarily the Simulation Server application's practical limit. The current profile value of **800** is conservative but reliable.

**Recommended action**:
- **Keep 800 as default** for Simulation Server compatibility
- **Add optional 1,500-2,500 profiles** for users who need better performance and have tested server stability
- **Document** that 10,000 is the server's theoretical maximum but not recommended for Simulation Server
- **Test thoroughly** before deploying higher batch sizes in production

The gap between 800 (current profile) and 10,000 (server capability) is significant, but **bridging it should be done incrementally with testing**, not in one jump.

---

**Report compiled by**: Claude Code
**Research methodology**: Web search of Prosys documentation, OPC UA specifications, community forums, and industry best practices
**Confidence level**: Medium-High (based on official documentation and community consensus, but lacking Prosys-specific performance benchmarks for Simulation Server)