# Container Layout



```
/data
 ├─ config.yaml           # See also configuration reference
 ├─ logs/                 # Rolling logs for agent, every data flow, Redpanda …
 ├─ redpanda/             # Redpanda data & WALs (backup-worthy)
 ├─ services/             # S6 service directories (only when S6_PERSIST_DIRECTORY=true)
 └─ hwid                  # Device fingerprint sent to the console

/tmp/umh-core-services/   # S6 service directories (default, cleared on restart)

/run/service/             # S6 scan directory (contains symlinks to service directories)
```

Mount **one persistent volume** (e.g. `umh-core-data`) to `/data` and you're done.

#### Advanced: Custom Data Location

If you need control over the exact data location (e.g., for compliance or backup requirements), you can use a custom folder instead of a Docker volume:

```bash
mkdir -p /path/to/umh-core-data
# Ensure container user has write access
chown -R 1000:1000 /path/to/umh-core-data
docker run -d --name umh-core -v /path/to/umh-core-data:/data:z ...
```

On SELinux systems (RHEL, Rocky), the `:z` flag allows Docker to relabel the directory. It's harmlessly ignored on other systems.

#### Upgrading Custom Folders to v0.44+

Version 0.44+ runs as a non-root user. If upgrading from an older version with a custom data folder, fix permissions first:

```bash
docker stop umh-core
docker rm umh-core

# Fix permissions for non-root container
sudo chown -R 1000:1000 /path/to/umh-core-data

docker run -d \
  --name umh-core \
  --restart unless-stopped \
  -v /path/to/umh-core-data:/data:z \
  management.umh.app/oci/united-manufacturing-hub/umh-core:<VERSION>
```

#### Migrating to Docker Volumes (Optional)

If you want to switch from a custom folder to a Docker volume:

```bash
docker stop umh-core
docker rm umh-core

# Create volume and copy data
docker volume create umh-core-data
docker run --rm \
  -v /path/to/umh-core-data:/source:ro,z \
  -v umh-core-data:/target \
  alpine sh -c "cp -av /source/. /target/"

# Fix permissions and start
docker run --rm -v umh-core-data:/data alpine chown -R 1000:1000 /data

docker run -d \
  --name umh-core \
  --restart unless-stopped \
  -v umh-core-data:/data \
  management.umh.app/oci/united-manufacturing-hub/umh-core:<VERSION>
```

Keep your old folder as backup for 24-48 hours before deleting.

### /config.yaml

See also [configuration-reference.md](configuration-reference.md "mention")

### /logs

| File/dir                                 | What it is for                                                                                 | When it appears                                                                                                     |
| ---------------------------------------- | ---------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| **`current`**                            | The file S6 is _actively_ appending log lines to. Keep an eye on this one with `tail -f`.      | Always – it is recreated immediately after every rotation. ([Skarnet](https://skarnet.org/software/s6/s6-log.html)) |
| **`previous`**                           | A temporary name used _during_ a rotation; disappears once rotation completes.                 | Only while a rotation is in flight. ([Skarnet](https://skarnet.org/software/s6/s6-log.html))                        |
| **`@<timestamp>.s`**                     | An archived log that was rotated _cleanly_. The timestamp is the moment the rotation occurred. | After every successful rotation. ([Skarnet](https://skarnet.org/software/s6/s6-log.html))                           |
| **`@<timestamp>.u`**                     | A “unfinished” archive – it was the `current` file when the container was killed.              | Only after an un-clean shutdown. ([Skarnet](https://skarnet.org/software/s6/s6-log.html))                           |
| `lock`, `state`, `processed`, `newstate` | Book-keeping files S6-log uses while rotating or while a post-processor runs.                  | Internal – you normally ignore them. ([Skarnet](https://skarnet.org/software/s6/s6-log.html))                       |

#### The life-cycle in practice

1. **Normal running** – all services write to their own `current` file.
2. **Size hits 1 MB** – S6 atomically renames `current` to a name such as `@20250530T131218Z.s`, then immediately creates a fresh empty `current`. ([Skarnet](https://skarnet.org/software/s6/s6-log.html))
3. **Prune** – if the directory now has > 20 archives, the oldest ones are deleted so the newest 20 remain. ([Skarnet](https://skarnet.org/software/s6/s6-log.html))
4.  **You read logs** – use:

    ```bash
    # live stream
    tail -f /data/logs/<service>/current

    # inspect an old file (the '@…s' ones are plain text)
    less /data/logs/<service>/@20250530T131218Z.
    ```

### /redpanda

The Redpanda data directory.

### HWID

A unique identifier for that UMH Core installation. Useful for troubleshooting.

### S6 Service Directories

UMH Core uses S6 overlay for service supervision. Service directories contain the runtime state and configuration for each managed service (Benthos, Redpanda, monitors, etc.).

By default, these directories are created in `/tmp/umh-core-services/` which is **cleared on container restart**, ensuring a clean state. This prevents issues from stale supervisor state files.

For debugging purposes, you can set `S6_PERSIST_DIRECTORY=true` to use `/data/services/` instead, which persists across container restarts. This allows inspection of S6 supervisor state files when troubleshooting service startup issues.

The `/run/service/` directory contains symlinks pointing to the actual service directories, which S6's scanner monitors for changes.

See the [Environment Variables](environment-variables.md) reference for more details on `S6_PERSIST_DIRECTORY`.
