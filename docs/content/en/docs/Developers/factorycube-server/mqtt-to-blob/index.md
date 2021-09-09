---
title: "mqtt-to-blob"
linkTitle: "mqtt-to-blob"
description: >
  The following guide describes how to catch data from the MQTT-Broker and push them to the blob storage from MIN.io
---

{{% notice warning %}}
This microservice is still in development and is not considered stable for production use.
{{% /notice %}}

## Getting started

The following guide describes how to catch data from the MQTT-Broker and push them to the blob storage from MIN.io
Go to the root folder of the project and execute the following command:

```bash
sudo docker-compose -f ./deployment/mqtt-to-blob/docker-compose.yml build && sudo docker-compose -f ./deployment/mqtt-to-blob/docker-compose.yml up 
```

## Environment variables

This chapter explains all used environment variables.

### BROKER_URL

**Description:** Specifies the DNS name / IP-address to connect to the MQTT Broker postgresql / timescaleDB 

**Type:** string

**Possible values:** all DNS names or IP 

**Example value:**  127.0.0.1

### BROKER_PORT

**Description:** Specifies the port for the MQTT-Broker. In most cases it is 1883. 

**Type:** int

**Possible values:** valid port number 

**Example value:** 1883

### MINIO_URL

**Description:** Specifies the database DNS name / IP-address for the MIN.io server. 

**Type:** string

**Possible values:** all DNS names or IP 

**Example value:**  play.min.io

### MINIO_ACCESS_KEY

**Description:** Specifies the key to access the MIN.io Server. Can be seen as the username to login.  

**Type:** string

**Possible values:** an existing / just created user with access to the specified database 

**Example value:**  testuser

### MINIO_SECRET_KEY

**Description:** Specifies the MIN.io Server password that should be used 

**Type:** string

**Possible values:** all

**Example value:**  changeme

### TOPIC

**Description:** Specifies the topic the MQTT will listen to. To subscribe to all topics use '#' instead of the topic. 

**Type:** string

**Possible values:** all

**Example value:**  /test/umh

### BUCKET_NAME

**Description:** Specifies the location in MIN.io where data are stored in MIN.io on the online.

**Type:** string

**Possible values:** all

**Example value:**  testbucket

