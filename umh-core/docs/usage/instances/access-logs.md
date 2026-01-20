# Accessing Logs

UMH Core provides multiple ways to access logs depending on your deployment setup.

## Container Logs (Recommended)

The simplest way to access logs is via Docker's built-in logging:

```bash
# View recent logs
docker logs umh-core

# Follow logs in real-time
docker logs -f umh-core

# Show last 100 lines and follow
docker logs -f --tail 100 umh-core
```

This works regardless of how your `/data` volume is mounted.

## Log Files

UMH Core also writes logs to files inside the container at `/data/logs/umh-core/`. This is useful when you need:
- Log persistence across container restarts
- Access to rotated historical logs
- Integration with external log collectors

### With Bind Mounts

If you mount `/data` to a host directory:

```bash
docker run -v /path/on/host:/data umh-core
```

You can access logs directly on the host:

```bash
# Follow logs in real-time
tail -F /path/on/host/logs/umh-core/current
```

### With Docker Volumes

If you use Docker volumes:

```bash
docker run -v umh-data:/data umh-core
```

Access logs from inside the container:

```bash
# Enter the container
docker exec -it umh-core /bin/sh

# Follow logs
tail -F /data/logs/umh-core/current
```

## Log File Structure

Logs are managed by S6's `s6-log` with automatic rotation:

| File | Description |
|------|-------------|
| `current` | Active log file |
| `@*.s` | Rotated archives (clean rotation) |
| `@*.u` | Unfinished archives (container was killed) |

Rotation settings:
- **Max file size**: 10 MB
- **Max archived files**: 5
- **Timestamp format**: TAI64N (convert with `tai64nlocal`)

## Timestamps

Log files use TAI64N timestamps (e.g., `@4000000067890abc12345678`). To convert to human-readable format, you can use the [tai64nlocal program](https://cr.yp.to/daemontools/tai64nlocal.html)

```bash
# Single command
tai64nlocal < /data/logs/umh-core/current

# Combined with tail
tail -F /data/logs/umh-core/current | tai64nlocal
```

## Troubleshooting

**No logs appearing in `docker logs`**: Ensure you're running a version that includes container log output (v0.45.0+).

**Log files not accessible**: With Docker Desktop on Mac/Windows, volumes are stored inside a VM. Use `docker logs` or `docker exec` instead of direct file access.

## Next Steps

- [Configuration File](config-file.md) - Edit instance configuration
- [State Machines](../../reference/state-machines.md) - Understand component states in logs
