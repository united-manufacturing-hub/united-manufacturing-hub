# Setup Guide

This guide walks you through setting up umh-core with Docker Compose, starting from a minimal configuration you can extend with [additional services](additional-services/README.md).

## What you'll need

- [Docker](https://docs.docker.com/get-started/introduction/get-docker-desktop/) installed on your system
- Basic familiarity with the command line

## Minimal Setup

1. Find the latest version on the [Releases](https://github.com/united-manufacturing-hub/united-manufacturing-hub/releases) page and replace `<VERSION>` with your selected version.
2. Save this in `docker-compose.yaml`.

> **Hint:** If your desired release is `v0.44.31`, the image tag is: `umh-core:v0.44.31`

```yaml
services:
  umh:
    # TODO: set your desired container version here 
    # e.g. `umh-core:v0.44.31`
    image: management.umh.app/oci/united-manufacturing-hub/umh-core:ENTER_VERSION_HERE
    restart: unless-stopped
    volumes:
      - umh-data:/data
    environment:
      # TODO: Enter your instance's Auth Token.
      # You'll find it in your instance's
      # configuration file on management.umh.app.
      - AUTH_TOKEN=your_auth_token
      # TODO: Change the LOCATION_0 parameter
      # to your desired Level 0 Location name
      - LOCATION_0=your_level_0_location
      # Optional: Define more levels
      # - LOCATION_1=your_level_1
      # - LOCATION_2=your_level_2
      # - LOCATION_3=your_level_3
      # - LOCATION_4=your_level_4
      - RELEASE_CHANNEL=stable
      - API_URL=https://management.umh.app/api

volumes:
  umh-data: {}
```

This achieves the same result as the docker cli commands, but the configuration is now documented in a file that you can version control and extend.

## Starting the Stack

Once the configuration is done you can start the stack:

```bash
docker compose up -d
```

We recommend that you familiarise yourself with the docker commands below. They'll come in handy while working with umh-core.

- `docker compose ps`: check the running state of all services
- `docker compose stats`: check resource usage of all services
- `docker compose pull`: attempt to pull new versions of all images used in the `docker-compose.yaml`

For more refer to the [official Docker Compose documentation](https://docs.docker.com/compose/).

## Add More Services

While umh-core can operate standalone, most deployments benefit from persistent storage and visualization capabilities.

- [Recommended UMH Stack](additional-services/recommended-umh-stack.md): umh-core, Grafana, PgBouncer, and TimescaleDB in one file
- [TimescaleDB](additional-services/timescaledb.md): time-series storage, with PgBouncer in front of it
- [Grafana](additional-services/grafana.md): create dashboards that run locally from your data
- [nginx](additional-services/nginx.md): reverse proxy and SSL termination
