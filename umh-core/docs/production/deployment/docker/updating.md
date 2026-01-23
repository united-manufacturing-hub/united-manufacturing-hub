# Updating

To update umh-core, stop the container and start a new one with the latest image.

Find the latest version on the [Releases](https://github.com/united-manufacturing-hub/united-manufacturing-hub/releases) page and replace `<VERSION>` with your selected version.

```bash
docker stop umh-core
docker rm umh-core

docker run -d \
  --name umh-core \
  --restart unless-stopped \
  --volume umh-core-data:/data \
  management.umh.app/oci/united-manufacturing-hub/umh-core:<VERSION>
```

**That's it!** Your data is preserved in the `umh-core-data` volume.

> **Note:** On Linux without Docker group membership, prefix commands with `sudo`.

---

> **Using a custom data folder?** If you manually specified a folder path instead of using a Docker volume, see [Container Layout](../reference/container-layout.md#advanced-custom-data-location) for upgrade instructions.
