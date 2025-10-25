# CSE Architecture

## Deployment Architecture vs Data Architecture

### Physical Deployment (3-tier)

```
Frontend (Browser)
    ↕ HTTPS
Relay (Cloud Proxy)
    ↕ HTTPS
Edge (umh-core at customer site)
```

### Logical Data Flow (2-tier)

```
Frontend ↔ Edge
(relay is transparent)
```

## Why Relay Exists

The relay is a **transparent proxy** that provides:

- **NAT traversal**: Customer sites are behind firewalls
- **HTTPS termination**: TLS offloading
- **Authentication**: Token validation, routing
- **E2E encryption**: Messages encrypted end-to-end

**Critical: Relay CANNOT see message contents** (E2E encrypted)

## Why We Coded It Wrong Initially

The original implementation treated relay as a stateful sync tier that processes data. This happened because:

1. **Misunderstood Linear's architecture**: Thought Linear had a relay tier
2. **Confused physical deployment with logical data flow**: 3 physical tiers ≠ 3 data tiers
3. **E2E encryption requirement came later**: Original design didn't account for relay being blind

From ENG-3622 analysis:

- Linear uses 2-tier sync (Client ↔ Server)
- UMH relay is like nginx/Cloudflare - it routes packets, nothing more
- E2E encryption means relay cannot read, transform, validate, or merge data

## Correct Mental Model

**Think of relay as invisible infrastructure:**

- Like your ISP's routers
- Like Cloudflare edge nodes
- Like nginx reverse proxy

**Sync logic is ONLY between Frontend and Edge:**

- Frontend tracks `lastSyncID` (what it has received)
- Edge tracks `latestSyncID` (what it can provide)
- Frontend → Edge: "Send changes after syncID X"
- Edge → Frontend: "Here are changes X+1 to Y"

**Relay just forwards encrypted packets** - it's not a participant in sync logic.

## Sync Protocol (2-tier)

### Frontend → Edge

```json
{
  "lastSyncID": 12345,
  "subscriptions": [
    {"collection": "container_identity", "filter": {}},
    {"collection": "container_desired", "filter": {"id": "worker-123"}}
  ]
}
```

### Edge → Frontend

```json
{
  "changes": [
    {"syncID": 12346, "collection": "container_observed", "op": "update", "doc": {...}},
    {"syncID": 12347, "collection": "container_desired", "op": "update", "doc": {...}}
  ],
  "latestSyncID": 12347
}
```

## References

- **Linear's sync engine**: [wzhudev/reverse-linear-sync-engine](https://github.com/wzhudev/reverse-linear-sync-engine)
- **CSE RFC**: ENG-3622
- **FSM v2 RFC**: ENG-3647

## Warning for Future Developers

**DO NOT implement relay state tracking again!**

If you're tempted to add:

- `relaySyncID` field
- `pendingRelay` queue
- `TierRelay` tier tracking

**STOP and ask:**

1. Would this work if relay is just nginx?
2. Can E2E encrypted relay actually see this data?
3. Did I read this ARCHITECTURE.md file?

The relay is blind. Keep it that way.
