# Restart an instance

Restarting an instance stops its container and starts it again with the same image, environment and persistent storage. This page covers the whole instance, not a bridge, flow or service.

Restarting an instance requires access to the system used to manage the UMH Core deployment and permission to restart it. This may be a container runtime (such as [Docker](docker/README.md) or [Docker Compose](docker-compose/README.md)), an orchestrator (such as Kubernetes), or a management interface (such as Portainer). If IT manages the deployment, ask them to perform the restart.

{% hint style="warning" %}
A restart stops every bridge and flow this instance runs. It collects nothing until its bridges and flows have recovered, and values your devices send in that time may be lost. The devices keep running, and other instances keep collecting. Only [Redundant Data Collection](../architecture-patterns/redundant-data-collection.md) keeps collection running, and it needs redundant PLCs. Plan a maintenance window.
{% endhint %}

## Restart the instance

1. Identify the system that manages this instance and the specific deployment to restart. If someone else deployed it, ask them. A wrong target, or a restart policy you did not expect, makes the restart do something you did not intend.
2. Use that system's restart procedure. These cover the common cases, and a management interface such as Portainer may drive one of them:
   - Docker: the [`docker restart`](https://docs.docker.com/reference/cli/docker/container/restart/) documentation.
   - Docker Compose: the [`docker compose restart`](https://docs.docker.com/reference/cli/docker/compose/restart/) documentation. If you deployed UMH Core with our [Docker Compose setup](docker-compose/setup.md), run `docker compose restart umh` in the directory that holds its `docker-compose.yaml`, the Compose project that runs the instance. For a custom setup, use its actual service name and project. Always give the service name: without it, the command restarts every service in the project, including any databases and dashboards.
   - Kubernetes, Docker Swarm, Portainer and other systems restart containers their own way; use their documentation.

## What a restart does not do

A restart does not apply a change the container reads at startup, such as a changed environment variable. Applying it means removing the container and starting a new one, which is a recreation.

A restart does not change the image. To move the instance to a new version, see [Updating with Docker](docker/updating.md) or [Updating with Docker Compose](docker-compose/updating.md).
