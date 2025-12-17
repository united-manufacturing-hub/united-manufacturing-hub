# Updating

To update umh-core, simply stop the container, pull the latest image, and start the container again.

> ⚠️ **For v0.44+:** umh-core now runs as non-root user (UID 1000). If upgrading from an older version, you must fix directory permissions before starting the new container.

## Standard Update (v0.44+)

```bash
# stop + delete the old container (data is preserved in the named volume)
docker stop umh-core
docker rm umh-core

# Fix permissions for non-root container
sudo chown -R 1000:1000 "$(pwd)/umh-core-data"

# pull the latest image and re-create
docker run -d \
  --name umh-core \
  --restart unless-stopped \
  -v umh-core-data:/data \
  management.umh.app/oci/united-manufacturing-hub/umh-core:<NEW_VERSION>
```

> **Note:** On Linux without Docker group membership, prefix commands with `sudo`.

Need to roll back? Just start the previous tag against the same `umh-core-data` volume.

## Automatic Permission Check

Starting with v0.44, umh-core checks `/data` permissions on startup. If permissions are incorrect, you'll see a clear error message with the exact `chown` command to run.

You can find the latest version on the [Releases](https://github.com/united-manufacturing-hub/united-manufacturing-hub/releases) page.
