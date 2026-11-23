# Restart an instance

Restarting an instance stops its container and starts it again with the same image, environment and persistent storage. This page covers the whole instance, not a bridge, flow or service.

{% hint style="warning" %}
A restart stops every bridge and flow this instance runs, so it collects nothing until it is back. [High Availability](../high-availability.md) shortens that gap but does not close it — a container restart takes 30-60 seconds. Only [Redundant Data Collection](../architecture-patterns/redundant-data-collection.md) keeps collection running, and it needs redundant PLCs. Plan a maintenance window.
{% endhint %}

## Restart the instance

1. Find out how the instance was started. `docker ps` and `docker compose ls` show what runs where; if someone else deployed it, ask them. A wrong name, or a restart policy you did not expect, makes the restart do something you did not intend.
2. Use that manager's restart procedure:
   - Docker: the [`docker restart`](https://docs.docker.com/reference/cli/docker/container/restart/) documentation.
   - Docker Compose: the [`docker compose restart`](https://docs.docker.com/reference/cli/docker/compose/restart/) documentation.
   - Kubernetes, Docker Swarm, Portainer and other managers restart containers their own way; use their documentation.

## What a restart does not do

A restart does not apply a change the container reads at startup, such as a changed environment variable. Applying it means removing the container and starting a new one, which is a recreation.

A restart does not change the image. To move the instance to a new version, see [Updating with Docker](docker/updating.md) or [Updating with Docker Compose](docker-compose/updating.md).
