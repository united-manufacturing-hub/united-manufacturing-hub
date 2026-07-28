# Grafana

In this example you'll learn how to add Grafana to a umh-core Docker Compose stack. If you're not sure about umh-core and Docker Compose stacks, start with [Setup](../setup.md).

[Grafana](https://grafana.com/) is an open-source visualization platform. It allows you to build dashboards showing real-time and historical data based on your umh-core configuration.

## Adding Grafana

Below are the changes to be made to the minimal configuration to deploy Grafana together with umh-core.

```diff
  services:
    umh:
      image: management.umh.app/oci/united-manufacturing-hub/umh-core:<VERSION> # TODO: change this
      restart: unless-stopped
      volumes:
        - umh-data:/data
      environment:
        - AUTH_TOKEN=your-auth-token # TODO: change this
        - LOCATION_0=your-location # TODO: change this
        - RELEASE_CHANNEL=stable
        - API_URL=https://management.umh.app/api

+   grafana:
+     image: management.umh.app/oci/grafana/grafana:12.3.0
+     restart: unless-stopped
+     ports:
+       - 3000:3000
+     environment:
+       # TODO: set your grafana password & username here
+       - GF_SECURITY_ADMIN_USER=admin
+       - GF_SECURITY_ADMIN_PASSWORD=admin
+     volumes:
+       - grafana-data:/var/lib/grafana
+     healthcheck:
+       test: ["CMD-SHELL", "curl --fail http://grafana:3000/api/health"]
+       interval: 10s
+       timeout: 5s
+       retries: 3

  volumes:
    umh-data: {}
+   grafana-data: {}
```

Grafana's admin account defaults to `admin` / `admin`. Port 3000 is published to the host, so set both values before you start the stack. The variable names follow Grafana's `GF_<SECTION>_<KEY>` convention, documented in the [Grafana configuration reference](https://grafana.com/docs/grafana/latest/setup-grafana/configure-grafana/).

## Complete docker-compose.yaml

umh-core with Grafana. Copy this into `docker-compose.yaml` and fill in the fields marked `TODO`.

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
      # TODO: Enter your instance's Auth Token. You'll find it in your instance's configuration file on management.umh.app.
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

  grafana:
    image: management.umh.app/oci/grafana/grafana:12.3.0
    restart: unless-stopped
    ports:
      - 3000:3000
    environment:
      # TODO: Set your desired username and password here
      # You'll need these credentials to access your local Grafana instance
      - GF_SECURITY_ADMIN_USER=admin     
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana-data:/var/lib/grafana
    healthcheck:
      test: ["CMD-SHELL", "curl --fail http://grafana:3000/api/health"]
      interval: 10s
      timeout: 5s
      retries: 3

volumes:
  umh-data: {}
  grafana-data: {}
```

## Connecting to Grafana
Once the stack is running, Grafana is reachable at `http://localhost:3000`.

To log into Grafana, use the credentials that you defined in `docker-compose.yml`.

> 💡 Without a persistent database behind it there is nothing to query, 
> so most deployments run Grafana together with TimescaleDB. We call it the **[Recommended UMH Stack](recommended-umh-stack.md)**.
