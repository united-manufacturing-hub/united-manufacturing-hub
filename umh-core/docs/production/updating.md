# Updating

To update umh-core, simply stop the container, pull the latest image, and start the container again.

```bash
# stop + delete the old container (data is preserved in the named volume)
docker stop umh-core
docker rm umh-core

# pull the latest image and re-create
docker run -d \
  --name umh-core \
  --restart unless-stopped \
  -v umh-core-data:/data \
  management.umh.app/oci/united-manufacturing-hub/umh-core:<NEW_VERSION>
```

> **Note:** On Linux without Docker group membership, prefix commands with `sudo`.

Need to roll back? Just start the previous tag against the same `umh-core-data` volume.

You can find the latest version on the [Releases](https://github.com/united-manufacturing-hub/united-manufacturing-hub/releases) page.
