---
title: "factoryinput"
linktitle: "factoryinput"
description: Documentation of factoryinput
aliases:
  - /docs/Developers/factorycube-server/factoryinput
  - /docs/developers/factorycube-server/factoryinput
---

**This microservice is still in development and is not considered stable for production use.**

This program provides an REST endpoint, to send MQTT messages via HTTP requests. It is typically accessed via [grafana-proxy](../grafana-proxy).

## Environment variables

This chapter explains all used environment variables.

### FACTORYINPUT_USER

**Description:** Specifies the admin user for the REST API 

**Type:** string

**Possible values:** all

**Example value:**  jeremy

### FACTORYINPUT_PASSWORD

**Description:** Specifies the password for the admin user for the REST API 

**Type:** string

**Possible values:** all

**Example value:**  changeme

### VERSION

**Description:** The version of the API to host. Currently, only 1 is a valid value

**Type:** int

**Possible values:** all

**Example value:**  1

### CERTIFICATE_NAME

**Description:** Certificate for MQTT authorization or NO_CERT

**Type:** string

**Possible values:** all

**Example value:** NO_CERT 

### MY_POD_NAME

**Description:** The pod name. Used only for tracing, logging and  MQTT client id. 

**Type:** string

**Possible values:** all 

**Example value:** app-factoryinput-0 

### BROKER_URL 

Description: the URL to the broker. Can be prepended either with ssl:// or mqtt:// or needs to specify the port afterwards with :1883 

Type: string

Possible values: all

Example value: tcp://united-manufacturing-hub-vernemq-local-service:1883

### CUSTOMER_NAME_{NUMBER}

**Description:** Specifies a user for the REST API 

**Type:** string

**Possible values:** all

**Example value:**  jeremy

### CUSTOMER_PASSWORD_{NUMBER}

**Description:** Specifies the password for the user for the REST API 

**Type:** string

**Possible values:** all

**Example value:**  changeme

### LOGGING_LEVEL

**Description:** Specifies the logging level. Everything except DEVELOPMENT will be parsed as production (including not set)

**Type:** string

**Possible values:** DEVELOPMENT, PRODUCTION

**Example value:**  PRODUCTION


### MQTT_QUEUE_HANDLER

**Description:** Number of queue workers to spawn. If not set, it defaults to 10

**Type:** uint

**Possible values:** 0-65535

**Example value:**  10

## Other

2) Run the program
   - Either using `go run github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinput`
   - Or using the Dockerfile
      - Open a terminal inside `united-manufacturing-hub`
      - Run `docker build -f .\deployment\mqtt-to-postgresql\Dockerfile .`
      - Look for the image SHA 
        - Example: `=> => writing image sha256:11e4e669d6581df4cb424d825889cf8989ae35a059c50bd56572e2f90dd6f2bc`
      - `docker run SHA`
        - `docker run 11e4e669d6581df4cb424d825889cf8989ae35a059c50bd56572e2f90dd6f2bc -e VERSION=1 ....`



## Rest API 
{{< swaggerui src="/openapi/factoryinput.yml" >}}
