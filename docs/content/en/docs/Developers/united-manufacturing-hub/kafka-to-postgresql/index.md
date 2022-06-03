---
title: "kafka-to-postgresql"
linkTitle: "kafka-to-postgresql"
description: >
  This microservices consumes messages from a Kafka topic and writes them to a PostgreSQL database.
---

# Getting started

Kafka-to-PostgreSQL is a microservice that consumes messages from a Kafka topic and writes them to a PostgreSQL database.

By default, it sets up, two kafka consumers, one for high throughput and one for high integrity.

## High throughput

This kafka listener is usually configured to listen on the [processValue](https://docs.umh.app/docs/concepts/mqtt/#processvalue) topics.

## High integrity

This kafka listener is usually configured to listen on all other topics.

# Environement variables

The following environment variables are used by the microservice:

| Variable                           | Type    | Description                                                                         |
|------------------------------------|---------|-------------------------------------------------------------------------------------|
| LOGGING_LEVEL                      | String  | Configures the zap logging level, set to DEVELOPMENT to enable development logging. |
| DRY_RUN                            | Boolean | If set to true, the microservice will not write to the database.                    | 
| POSTGRES_HOST                      | String  | The hostname of the PostgreSQL database.                                            |
| POSTGRES_USER                      | String  | The username to use for PostgreSQL connections.                                     |
| POSTGRES_PASSWORD                  | String  | The password to use for PostgreSQL connections.                                     |
| POSTGRES_DATABASE                  | String  | The name of the PostgreSQL database.                                                |
| POSTGRES_SSLMODE                   | Boolean | If set to true, the PostgreSQL connection will use SSL.                             |
| KAFKA_BOOTSTRAP_SERVERS             | String  | The kafka server to connect to.                                                     |
| KAFKA_HIGH_INTEGRITY_LISTEN_TOPIC  | String  | The kafka topic to listen to for high integrity messages. (This can be a regex)     |
| KAFKA_HIGH_THROUGHPUT_LISTEN_TOPIC | String  | The kafka topic to listen to for high throughput messages. (This can be a regex)    |


# Program flow

The graphic below shows the program flow of the microservice.

![Kafka-to-postgres-flow](kafka-to-postgresql-flow.drawio.svg)

# Data flow

## High Integrity
The graphic below shows the flow for an example High Integrity message.

![Kafka-hi-data-flow](HICountFlow.drawio.svg)