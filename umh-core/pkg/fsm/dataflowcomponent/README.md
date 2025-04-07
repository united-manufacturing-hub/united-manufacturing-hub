# DataFlowComponent FSM

## Overview
This package implements the Finite State Machine (FSM) for DataFlowComponent services. Each DataFlowComponent service creates its own BenthosManager, which in turn creates its own PortManager for allocating metrics ports to Benthos instances.

## Port Management Between Multiple DataFlowComponent Services

### Problem
When multiple DataFlowComponent services are created, each with its own BenthosManager, they all independently allocate ports from the same default range (9000-9999). This can lead to port conflicts when different DataFlowComponent services try to use the same ports for their Benthos instances.

### Solution
To avoid port conflicts, implement one of these strategies:

#### 1. Use Different Port Ranges (Simple but not recommended for production)
Assign different port ranges to each DataFlowComponent service.

```go
// Create first DFC service with ports 9000-9499
dfc1 := dataflowcomponent.NewDefaultDataFlowComponentService(
    "component1", 
    dataflowcomponent.WithPortRange(9000, 9499),
)

// Create second DFC service with ports 9500-9999
dfc2 := dataflowcomponent.NewDefaultDataFlowComponentService(
    "component2", 
    dataflowcomponent.WithPortRange(9500, 9999),
)
```

#### 2. Use a Shared PortManager (Recommended)
Create a single shared PortManager instance and pass it to all DataFlowComponent services:

```go
// Create a shared port manager
sharedPortManager, err := portmanager.NewDefaultPortManager(9000, 9999)
if err != nil {
    // Handle error
}

// Create DFC services with the shared port manager
dfc1 := dataflowcomponent.NewDefaultDataFlowComponentService(
    "component1", 
    dataflowcomponent.WithSharedPortManager(sharedPortManager),
)

dfc2 := dataflowcomponent.NewDefaultDataFlowComponentService(
    "component2", 
    dataflowcomponent.WithSharedPortManager(sharedPortManager),
)
```

### Implementation Notes

- The shared PortManager approach is the recommended solution as it ensures proper coordination of port allocation.
- This is particularly important in production environments where multiple DataFlowComponent services might be running.
- When implementing the DataFlowComponent FSM manager, ensure that it properly handles the shared PortManager.
- The PortManager needs to be thread-safe as it will be accessed by multiple DataFlowComponent services concurrently.

### Future Enhancements

When implementing the DataFlowComponentManager, consider these enhancements:

1. Add support for a configurable shared PortManager at the manager level
2. Implement port persistence to survive restarts
3. Consider adding health checks to verify that allocated ports are actually free
4. Add monitoring to track port allocation/utilization 