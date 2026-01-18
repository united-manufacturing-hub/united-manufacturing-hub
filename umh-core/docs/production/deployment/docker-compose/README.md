# Docker Compose

This documentation describes how to deploy umh-core using Docker Compose as an alternative to the standard `docker run` command mentioned in the [Getting Started section](../../../getting-started/README.md).

## Why Docker Compose?

While umh-core can be started with a single `docker run` command, real-world deployments often require additional services such as databases, visualization tools, and reverse proxies. Docker Compose allows you to define and manage multi-container applications in a single, declarative YAML file. Compared to using the Docker CLI alone, this is easier to set up and maintain.

If you are unfamiliar with Docker Compose, refer to the [official Docker Compose documentation](https://docs.docker.com/compose/) for a comprehensive introduction.

## What is Docker Compose

Docker Compose uses YAML files for the declaration of Docker Resources. These files are called `docker-compose.yaml`.

In these files Docker Volumes, Docker Networks and Docker Containers can all be declared and configured to work with each other. This makes Docker Compose particularly valuable when additional Services like Grafana or TimescaleDB should be deployed alongside umh-core. But it is also valuable if only umh-core is deployed as it keeps the configuration of the container in a file instead of being lost to a one-off command.
