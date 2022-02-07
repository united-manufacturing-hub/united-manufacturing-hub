---
title: "mqtt-kafka-bridge"
linkTitle: "mqtt-kafka-bridge"
description: >
  This docker container transfers messages from mqtt to kafka and vise versa
aliases:
  - /docs/Developers/factorycube-server/mqtt-kafka-bridge
  - /docs/developers/factorycube-server/mqtt-kafka-bridge
---

## Getting started

### Using the Helm chart

By default, mqtt-kafka-bridge will be deactivated in united-manufacturing-hub.
## Environment variables

This chapter explains all used environment variables.

### MQTT_BROKER_URL

**Description:** The MQTT broker URL

**Type:** string

**Possible values:** IP, DNS name

**Example value:** ia_mosquitto

**Example value 2:** united-manufacturing-hub-vernemq-local-service:1883

### MQTT_CERTIFICATE_NAME

**Description:** Set to NO_CERT to allow non-encrypted MQTT access

**Type:** string

**Possible values:** NO_CERT, any string

**Example value:** NO_CERT


### MQTT_TOPIC

**Description:** MQTT topic to listen on

**Type:** string

**Possible values:** any string

**Example value:** ia/#



### KAFKA_BOOSTRAP_SERVER

**Description:** The Kafka broker URL

**Type:** string

**Possible values:** IP, DNS name

**Example value:** united-manufacturing-hub-kafka:9092


### KAFKA_LISTEN_TOPIC

**Description:** Kafka topic to listen on

**Type:** string

**Possible values:** any string

**Example value:** ia/#


### KAFKA_BASE_TOPIC

**Description:** Kafka base topic (usually the top most part of the listen topic)

**Type:** string

**Possible values:** any string

**Example value:** ia

### REDIS_URI

**Description:** Redis database url

**Type:** string

**Possible values:** IP, DNS name

**Example value:** united-manufacturing-hub-redis-node-0.united-manufacturing-hub-redis-headless:26379

### REDIS_PASSWORD

**Description:** Redis password

**Type:** string

**Possible values:** any string

**Example value:** super secret password

### MY_POD_NAME

**Description:** Name of the docker pod, used for identification at the MQTT broker

**Type:** string

**Possible values:** any string

**Example value:** MQTT-Kafka-Bridge