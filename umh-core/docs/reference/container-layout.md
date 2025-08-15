# Container Layout



```
/data
 ├─ config.yaml           # See also configuration reference
 ├─ logs/                 # Rolling logs for agent, every data flow, Redpanda …
 ├─ redpanda/             # Redpanda data & WALs (backup-worthy)
 └─ hwid                  # Device fingerprint sent to the console
```

Mount **one persistent volume** (e.g. `umh-core-data`) to `/data` and you’re done.



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
    tail -f /logs/<service>/current

    # inspect an old file (the '@…s' ones are plain text)
    less /logs/<service>/@20250530T131218Z.
    ```

### /redpanda

The Redpanda data directory.

### HWID

A unique identifier for that UMH Core installation. Useful for troubleshooting.
