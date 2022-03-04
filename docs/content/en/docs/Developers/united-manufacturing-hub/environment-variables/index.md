---
title: "environment-variables"
linkTitle: "environment-variables"
description: >
  This site describes all environment variables used to setup and customize the united manufacturing hub.
aliases:
  - /docs/Developers/factorycube-edge/environment-variables
  - /docs/developers/factorycube-edge/environment-variables
---

## Overview

Environment variables are used to setup and customize your system to your needs. The United Manufacturing Hub has a lot of different microservices you can enable or disable via environment variables. Additionally those microservices have settings you can set with those environment variables.

This webpage shows you how to change them and what variables are accessible.

## How to change environment variables
The environment variables are specified in a .yaml file. You can either directly change them in the file or use our Management SaaS tool to generate the file according to your needs. The latter option eliminates the risk of invalid .yaml files.

## Table of environment variables
| Path | Description| Type | Possible values | Example value | Example value 2 |
| ---  | --- | --- | --- | --- | --- |
| _000_commonConfig/datasources/sensorconnect/enabled | Enables or disables sensorconnect microservice. | bool | true, false | true
| _000_commonConfig/datasources/sensorconnect/iprange | The IP range to search for ifm gateways. | string | All subnets in [CIDR notation](https://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing) | 172.16.0.0/24
| TRANSMITTERID | The unique transmitter id. This will be used for the creation of the MQTT/Kafka topic. ia/raw/TRANSMITTERID/... | string | all | 2021-0156 | development |
| sensorconnect/ioddfilepath | A persistent path, to save downloaded IODD files at | string | All valid paths | /tmp/iodd
| sensorconnect/lowerPollingTime | The fastest time to read values from connected sensors in milliseconds | int | all | 100
| sensorconnect/upperPollingTime | The slowest time to read values from connected sensors in milliseconds. To disable this feature, set this variable to the same value as lowerPollingTime | int | all | 100
| sensorconnect/pollingSpeedStepUpMs | The time to add to the actual tick rate in case of a failure (incremental) | int | all | 10
| sensorconnect/pollingSpeedStepDownMs | The time to subtract from actual tick rate in case of a failure (incremental) | int | all | 10 
| sensorconnect/sensorInitialPollingTimeMs | The tick speed, that sensor connect will start from. Set a bit higher than LOWER_POLLING_TIME_MS to allow sensors to recover from faults easier | int | all | 100
| sensorconnect/maxSensorErrorCount | After this numbers on errors, the sensor will be removed until the next device scan  | int | all | 50 | 
| DEVICE_FINDER_TIME_SEC | Seconds between scanning for new devices | int | all | 10
| deviceFinderTimeoutSec | Timeout per device for scan response | int | all | 10 
| sensorconnect/additionalSleepTimePerActivePortMs | This adds a sleep per active port on an IO-Link master | float | all | 0.0
| sensorconnect/allowSubTwentyMs | Allows query times below 20MS. DO NOT activate this, if you are unsure, that your devices survive this load. | bool | 0, 1 | 0
| sensorconnect/additionalSlowDownMap | A json map of additional slowdowns per device. Identificates can be Serialnumber, Productcode or URL| int | all | [] | ```json[{"serialnumber":"000200610104","slowdown_ms":-10},{"url":"http://192.168.0.13","slowdown_ms":20},{"productcode":"AL13500","slowdown_ms":20.01}]```
| sensorconnect/debug | Set to 1 to enable debug output | bool | 0, 1| 0
| sensorconnect/resources/requests/cpu | CPU usage in [milliCPU](https://kubernetes.io/docs/tasks/configure-pod-container/assign-cpu-resource/#cpu-units) | string | 1.5m
| USE_MQTT | Enables sending using MQTT | bool | true, false, 0, 1 | true
| MQTT_BROKER_URL | The MQTT broker URL (with port) | string | IP, DNS name | united-manufacturing-hub-vernemq-local-service:1883 | localhost:1883
| MQTT_CERTIFICATE_NAME | The name of the certificate to use | string | all | NO_NAME
| USE_KAFKA | Enables sending using Kafka | bool | true, false, 0, 1 | true
| MQTT_BROKER_URL | The Kafka boostrap server url | string | IP, DNS name | united-manufacturing-hub-kafka:9092
