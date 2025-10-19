# pkg/cse/sync

## Sync Orchestration Layer

Synchronization orchestration for CSE's 2-tier architecture (Frontend ↔ Edge).

## Architecture WARNING

**DEPLOYMENT**: Frontend ↔ Relay ↔ Edge (3 physical tiers)
**DATA FLOW**: Frontend ↔ Edge (2 logical tiers)

### Relay is a TRANSPARENT PROXY

The relay is **E2E encrypted** and **cannot see message contents**.

Think of relay like:
- nginx reverse proxy
- Cloudflare edge node
- Your ISP's routers

**Relay provides**:
- NAT traversal (customer sites behind firewalls)
- HTTPS termination / TLS offloading
- Authentication / routing
- Connection management

**Relay does NOT**:
- Read data (E2E encrypted)
- Transform data (blind to contents)
- Validate data (cannot see it)
- Merge changes (no sync logic)
- Maintain sync state (not a tier)

See `../storage/ARCHITECTURE.md` for complete explanation.

## SyncState Component

2-tier sync state tracking for Frontend ↔ Edge synchronization.

**Key Features**:
- Delta sync based on `_sync_id` monotonic counter
- Subscription-based (client declares what to sync)
- Optimistic concurrency via `_version` field
- Inspired by Linear's sync engine

**Testing**: 29 specs verify state transitions and delta queries

## Dependencies

- **Internal**: `pkg/cse/storage` (persistence), `pkg/cse/protocol` (transport)
- **External**: Context management

## Testing

```bash
ginkgo -r ./pkg/cse/sync
```

Current: 29 specs passing

## References

- **Linear sync engine**: wzhudev/reverse-linear-sync-engine
- **CSE RFC (ENG-3622)**: Complete design documentation
- **Storage ARCHITECTURE.md**: Why relay is transparent (prevent confusion)
