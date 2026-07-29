# Updating

To update umh-core, edit the image tag in your `docker-compose.yaml` and restart the stack.

1. Find the latest version on the [Releases](https://github.com/united-manufacturing-hub/united-manufacturing-hub/releases) page and replace `ENTER_VERSION_HERE` with your selected version.

2. Update the umh-core image tag in `docker-compose.yaml`:

```yaml
services:
  umh:
    # e.g umh-core:v0.44.31
    image: management.umh.app/oci/united-manufacturing-hub/umh-core:ENTER_VERSION_HERE
```

3. Pull the new image and restart:

```bash
docker compose up -d
```

**That's it!** Your data is preserved in the named volumes.

> **Note:** On Linux without Docker group membership, prefix commands with `sudo`.
