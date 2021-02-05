# sensorconnect

- [sensorconnect](#sensorconnect)
  - [Getting started](#getting-started)
  - [Environment variables](#environment-variables)
    - [TRANSMITTERID](#transmitterid)
    - [BROKER_URL](#broker_url)
    - [BROKER_PORT](#broker_port)
    - [IP_RANGE](#ip_range)

This docker container automatically detects ifm gateways in the specified network and reads their sensor values in the highest possible data frequency. The MQTT output is specified in [the MQTT documentation](../general/mqtt.md)

## Getting started

Here is a quick tutorial on how to start up a basic configuration / a basic docker-compose stack, so that you can develop.

1. Copy `deployment/sensorconnect/docker-compose.yml` to the main folder
2. Specify the environment variables, e.g. in a .env file in the main folder or directly in the docker-compose
3. execute `sudo docker-compose up -d`

## Environment variables

This chapter explains all used environment variables.

### TRANSMITTERID

Description: The unique transmitter id. This will be used for the creation of the MQTT topic. ia/raw/TRANSMITTERID/...

Type: string

Possible values: all

Example value: 2021-0156

### BROKER_URL

**Description:** The MQTT broker URL

**Type:** string

**Possible values:** IP, DNS name

**Example value:** ia_mosquitto

**Example value 2:** localhost

### BROKER_PORT

**Description:** The MQTT broker port. Only unencrypted ports are allowed here (default: 1883)

**Type:** integer

**Possible values:** all

**Example value:** 1883

### IP_RANGE

**Description:** The IP range to search for ifm gateways

**Type:** string

**Possible values:** All subnets in [CIDR notation](https://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing)

**Example value:** 172.16.0.0/24
