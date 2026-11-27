# Restart an instance

Restarting an instance with Docker or Docker Compose stops its container and starts it again with the same image, environment and persistent storage. This page covers the whole instance, not a bridge, flow or service.

Restarting an instance requires access to the system used to manage the UMH Core deployment and permission to restart it. This may be a container runtime (such as [Docker](docker/README.md) or [Docker Compose](docker-compose/README.md)), an orchestrator (such as Kubernetes), or a management interface (such as Portainer). If IT manages the deployment, ask them to perform the restart.

{% hint style="warning" %}
A restart stops every bridge and flow this instance runs. It collects nothing until its bridges and flows have recovered, and values your devices send in that time may be lost. The devices keep running, and other instances keep collecting. To keep collecting through a hardware failure, you need redundant PLCs; see [Redundant Data Collection](../architecture-patterns/redundant-data-collection.md). For a planned restart, that page says a single PLC with multiple addresses may avoid the gap, but it does not describe how. Plan a maintenance window.
{% endhint %}

## Restart the instance

1. Identify the system that manages this instance and the specific deployment to restart. If someone else deployed it, ask them. A wrong target, or a restart policy you did not expect, makes the restart do something you did not intend.
2. Use that system's restart procedure. These cover the common cases, and a management interface such as Portainer may drive one of them:
   - Docker: the [`docker restart`](https://docs.docker.com/reference/cli/docker/container/restart/) documentation.
   - Docker Compose: the [`docker compose restart`](https://docs.docker.com/reference/cli/docker/compose/restart/) documentation. If you deployed UMH Core with our [Docker Compose setup](docker-compose/setup.md), run `docker compose restart umh` in the directory that holds its `docker-compose.yaml`, the Compose project that runs the instance. For a custom setup, use its actual service name and project. Always give the service name: without it, the command restarts every service in the project, including any databases and dashboards.
   - Kubernetes, Docker Swarm, Portainer and other systems restart containers their own way; use their documentation.

## What a restart does not do

A Docker or Docker Compose restart stops and starts the same container. The container keeps the settings it was created with: environment variables, mounts, secret mounts and other creation flags. To change one of them, remove the container and create a new one. That is a recreation. Editing the contents of a mounted file does not need one.

When you recreate, give the new container the same configuration as the old one and change only what you intended to change. `docker inspect <container name>` lists the environment, mounts, networks and flags of the old container. Run it before you remove the container.

The new container must also mount the same `/data`. It holds the configuration, the Redpanda data and the logs. A container started with `docker run` does not reuse the anonymous volume of the old one, so it starts with an empty `/data`. If the old container ran with `--rm`, removing it deletes that volume. [Add your corporate CA to the container](../../usage/instances/communicator/instance-offline-behind-corporate-firewall.md#add-your-corporate-ca-to-the-container) shows how to find the volume and copy the data out before you remove the container.

A Docker or Docker Compose restart does not change the image. To move the instance to a new version, see [Updating with Docker](docker/updating.md) or [Updating with Docker Compose](docker-compose/updating.md). The Docker procedure uses the container name `umh-core`, the `umh-core-data` volume and no custom environment. If your instance uses a different name, volume or environment, adapt the commands so the new container keeps them. Otherwise, the new container can start without your configuration or data.
