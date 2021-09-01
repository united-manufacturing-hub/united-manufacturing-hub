---
title: "mqtt-to-postgresql"
linktitle: "mqtt-to-postgresql"
description: >
  Documentation of mqtt-to-postgresql
---

TODO: #80 fill out standardized documentation for mqtt-to-postgresql

# |NAME OF DOCKER CONTAINER|

|This is a short description of the docker container.|

## Getting started

Here is a quick tutorial on how to start up a basic configuration / a basic docker-compose stack, so that you can develop.

|TUTORIAL|

docker-compose -f ./deployment/mqtt-to-postgresql/docker-compose-mqtt-to-postgresql-development.yml --env-file ./.env up -d --build

## Environment variables

This chapter explains all used environment variables.

### POSTGRES_HOST

**Description:** Specifies the database DNS name / IP-address for postgresql / timescaleDB 

**Type:** string

**Possible values:** all DNS names or IP 

**Example value:**  factorycube-server

### POSTGRES_PORT

**Description:** Specifies the database port for postgresql 

**Type:** int

**Possible values:** valid port number 

**Example value:** 5432

### POSTGRES_DATABASE

**Description:** Specifies the database name that should be used 

**Type:** string

**Possible values:** an existing database in postgresql 

**Example value:**  factoryinsight

### POSTGRES_USER

**Description:** Specifies the database user that should be used 

**Type:** string

**Possible values:** an existing user with access to the specified database in postgresql 

**Example value:**  factoryinsight

### POSTGRES_PASSWORD

**Description:** Specifies the database password that should be used 

**Type:** string

**Possible values:** all

**Example value:**  changeme

### DRY_RUN

**Description:** Enables dry run mode (doing everything, even "writing" to database, except committing the changes.) 

**Type:** string

**Possible values:** true,false

**Example value:**  false

### REDIS_URI

**Description:** URI for accessing redis sentinel  

**Type:** string

**Possible values:** All valids URIs

**Example value:** factorycube-server-redis-node-0.factorycube-server-redis-headless:26379

### REDIS_URI2

**Description:** Backup URI for accessing redis sentinel  

**Type:** string

**Possible values:** All valids URIs

**Example value:** factorycube-server-redis-node-1.factorycube-server-redis-headless:26379

### REDIS_URI3

**Description:** Backup URI for accessing redis sentinel  

**Type:** string

**Possible values:** All valids URIs

**Example value:** factorycube-server-redis-node-2.factorycube-server-redis-headless:26379

### REDIS_PASSWORD

**Description:** Password for accessing redis sentinel  

**Type:** string

**Possible values:** all 

**Example value:** changeme 

### MY_POD_NAME

**Description:** Password for accessing redis sentinel  

**Type:** string

**Possible values:** all 

**Example value:** changeme 

### MQTT_TOPIC

**Description:** Topic to subscribe to. Only set for debugging purposes, e.g., to subscribe to a certain message type. Default usually works fine.  

**Type:** string

**Possible values:**  all possible MQTT topics 

**Example value:** $share/MQTT_TO_POSTGRESQL/ia/# 


