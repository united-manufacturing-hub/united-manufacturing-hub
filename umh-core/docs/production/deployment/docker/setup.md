# Setup

You can run umh-core without the Management Console. But you'll need to edit configuration files directly.

Find the latest version on the [Releases](https://github.com/united-manufacturing-hub/united-manufacturing-hub/releases) page and replace `<VERSION>` with your selected version.

```bash
docker volume create umh-core-data

docker run -d \
  --name umh-core \
  --restart unless-stopped \
  --env AUTH_TOKEN=your-token \
  --env LOCATION_0=your-location \
  --env RELEASE_CHANNEL=stable \
  --env API_URL=https://management.umh.app/api \
  --volume umh-core-data:/data \
  management.umh.app/oci/united-manufacturing-hub/umh-core:<VERSION>
```
