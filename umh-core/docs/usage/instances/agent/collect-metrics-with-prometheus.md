# Collect Metrics with Prometheus

This page explains how to read the metrics of a UMH Core instance and how to collect them with Prometheus. The steps need access to the Docker host and to your Prometheus setup.

Every instance serves its metrics on port 8080, at the path `/metrics`, in the Prometheus text format. The endpoint runs inside the container, so you can read it when the Management Console cannot reach the instance. [Metrics](metrics.md) explains the series.

## Read the metrics once

Run this on the machine that runs the container. The install command from the Management Console names the container `umh-core`:

```bash
docker exec <container name> wget -qO- http://localhost:8080/metrics
```

## Collect the metrics with Prometheus

1. Publish port 8080 of the container. The install command from the Management Console does not publish it. Remove the container and start it again with the same `docker run` command plus `-p <host-ip>:8080:8080`, where `<host-ip>` is an address of the host that Prometheus can reach, for example `127.0.0.1` when Prometheus runs on the same host. `docker restart` keeps the old port mappings, so it is not enough.

   The endpoint has no authentication. Without `<host-ip>`, Docker publishes the port on every interface of the host, so always give an address on a trusted network.

2. Add a scrape job to your Prometheus configuration:

   ```yaml
   scrape_configs:
     - job_name: umh-core
       static_configs:
         - targets: ["<host-ip>:8080"]
   ```

3. Decide how to handle the `instance` label. Prometheus sets its own `instance` label on every scrape. With the default `honor_labels: false`, it renames umh-core's `instance` label to `exported_instance`. In Prometheus queries, write `exported_instance="Core"` where the component pages write `instance="Core"`. Alternatively, set `honor_labels: true` in the scrape job.

The port inside the container is always 8080. umh-core sets `agent.metricsPort` in `config.yaml` back to 8080 on every start. To use another port on the host, change the host side of the mapping, for example `-p <host-ip>:9100:8080`.
