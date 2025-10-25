# SQLite WAL Checkpoint Handling

**Date**: 2025-01-19
**Context**: Task 8 of Important Improvements from reliability review

## Overview

SQLite WAL (Write-Ahead Logging) mode requires periodic checkpointing to flush
changes from the WAL file back to the main database file.

## Checkpoint Strategy

### Automatic Checkpoints

SQLite automatically checkpoints when:
- WAL file reaches 1000 pages (~4MB with 4KB page size)
- Database connection closes
- PRAGMA wal_checkpoint() executed

### Manual Checkpoint on Close()

Our implementation explicitly calls `PRAGMA wal_checkpoint(TRUNCATE)` during
`Close()` to ensure clean shutdown:

```go
if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
    // Log warning but don't fail close
    fmt.Printf("WARNING: WAL checkpoint failed during close: %v\n", err)
}
```

### Why TRUNCATE Mode?

- **PASSIVE**: Checkpoints only if no readers (may not checkpoint)
- **FULL**: Waits for readers to finish (may block)
- **RESTART**: Like FULL + resets WAL (may block)
- **TRUNCATE**: Like RESTART + truncates WAL file ✅

We use TRUNCATE to minimize disk usage and ensure WAL is flushed.

## Error Handling

### Checkpoint Can Fail

Common failure scenarios:
1. **Another process has database open**: Checkpoint blocked
2. **Long-running read transaction**: Checkpoint waits
3. **Filesystem issues**: I/O errors
4. **Context timeout**: Operation cancelled

### Graceful Degradation

When checkpoint fails:
- ✅ Warning logged to stderr
- ✅ Database still closes successfully
- ✅ No data loss (WAL contains transaction history)
- ✅ WAL automatically checkpointed on next open

**Key insight**: Checkpoint failure is not catastrophic. SQLite recovers
automatically on next database open.

## Production Implications

### Disk Space

Without successful checkpoints:
- WAL file grows unbounded
- Can reach multiple GB on busy systems
- Eventually triggers automatic checkpoint (but at 1000 pages)

### Recovery Time

If WAL grows large:
- Next database open takes longer (replays WAL)
- Typical: <1s for <100MB WAL
- Large: 5-10s for multi-GB WAL

### Monitoring

Monitor WAL file size:
```bash
ls -lh /data/state.db-wal
```

If consistently >100MB, investigate:
- Are there long-running transactions?
- Is another process holding the database?
- Are checkpoints succeeding?

## Testing

Manual test for checkpoint behavior:
```go
// Create store, insert data, close
cfg := basic.DefaultConfig("test.db")
store, _ := basic.NewStore(cfg)
store.CreateCollection(ctx, "test", nil)

for i := 0; i < 1000; i++ {
    store.Insert(ctx, "test", basic.Document{"idx": i})
}

// Check WAL size before close
info, _ := os.Stat("test.db-wal")
fmt.Printf("WAL size before close: %d bytes\n", info.Size())

// Close triggers checkpoint
store.Close(ctx)

// Check WAL size after close (should be small or deleted)
info, _ = os.Stat("test.db-wal")
fmt.Printf("WAL size after close: %d bytes\n", info.Size())
```

## References

- SQLite WAL Mode: https://www.sqlite.org/wal.html
- Checkpoint Documentation: https://www.sqlite.org/pragma.html#pragma_wal_checkpoint
- Go database/sql: https://pkg.go.dev/database/sql
