---
title: "sensorconnect"
linkTitle: "sensorconnect"
description: >
  This docker container automatically detects ifm gateways in the specified network and reads their sensor values in the highest possible data frequency.
---

## Getting started

Here is a quick tutorial on how to start up a basic configuration / a basic docker-compose stack, so that you can develop.

1. execute `docker-compose -f ./deployment/sensorconnect/docker-compose.yaml up -d --build`

## Environment variables

This chapter explains all used environment variables.

### TRANSMITTERID

**Description**: The unique transmitter id. This will be used for the creation of the MQTT/Kafka topic. ia/raw/TRANSMITTERID/...

**Type**: string

**Possible values**: all

**Example value**: 2021-0156

**Example value 2**: development

### IP_RANGE

**Description:** The IP range to search for ifm gateways

**Type:** string

**Possible values:** All subnets in [CIDR notation](https://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing)

**Example value:** 172.16.0.0/24


### IODD_FILE_PATH

**Description:** A persistent path, to save downloaded IODD files at

**Type:** string

**Possible values:** All valid paths

**Example value:** /tmp/iodd

### LOWER_SENSOR_TICK_TIME_MS

**Description:** The fastest time to read values from connected sensors in milliseconds

**Type:** int

**Possible values:** all

**Example value:** 100


### UPPER_SENSOR_TICK_TIME_MS

**Description:** The slowest time to read values from connected sensors in milliseconds

**Note:** To disable this feature, set this variable to the same value as LOWER_SENSOR_TICK_TIME_MS

**Type:** int

**Possible values:** all

**Example value:** 100

### SENSOR_TICK_STEP_MS_UP

**Description:** The time to add to the actual tick rate in case of a failure (incremental)

**Type:** int

**Possible values:** all

**Example value:** 10

### SENSOR_TICK_STEP_MS_UP

**Description:** The time to subtract from actual tick rate in case of a failure (incremental)

**Type:** int

**Possible values:** all

**Example value:** 10



#### MQTT only

### USE_MQTT

**Description:** Enables sending using MQTT

**Type:** bool

**Possible values:** true, false, 0, 1

### MQTT_BROKER_URL

**Description:** The MQTT broker URL (with port)

**Type:** string

**Possible values:** IP, DNS name

**Example value:** united-manufacturing-hub-vernemq-local-service:1883

**Example value 2:** localhost:1883

### MQTT_CERTIFICATE_NAME

**Description:** The name of the certificate to use

**Type:** string

**Possible values:** all

**Example value:** NO_NAME

#### Kafka only

### USE_KAFKA

**Description:** Enables sending using Kafka

**Type:** bool

**Possible values:** true, false, 0, 1

### MQTT_BROKER_URL

**Description:** The Kafka boostrap server url

**Type:** string

**Possible values:** IP, DNS name

**Example value:** united-manufacturing-hub-kafka:9092
