# Updating

To update umh-core, simply stop the container, pull the latest image, and start the container again.

```bash
# stop + delete the old container (data is preserved)
sudo docker stop umh-core
sudo docker rm umh-core

# pull the latest image and re-create
sudo docker run -d \
  --name umh-core \
  --restart unless-stopped \
  -v "$(pwd)/umh-core-data":/data \
  management.umh.app/oci/united-manufacturing-hub/umh-core:<NEW_VERSION>
```

Need to roll back? Just start the previous tag against the same `/data` volume.

You can find the latest version on the [Releases](https://github.com/united-manufacturing-hub/united-manufacturing-hub/releases) page.
