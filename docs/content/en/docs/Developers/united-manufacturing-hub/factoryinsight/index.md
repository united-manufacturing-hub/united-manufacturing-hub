---
title: "factoryinsight"
linkTitle: "factoryinsight"
description: >
  This document describes the usage of factoryinsight including environment variables and the REST API
aliases:
  - /docs/Developers/factorycube-server/factoryinsight
  - /docs/developers/factorycube-server/factoryinsight
---

## Getting started

Here is a quick tutorial on how to start up a basic configuration / a basic docker-compose stack, so that you can develop.

Go to the root folder of the project and execute the following command:

```bash
sudo docker build -f deployment/factoryinsight/Dockerfile -t factoryinsight:latest . && sudo docker run factoryinsight:latest 
```

## Environment variables

This chapter explains all used environment variables.

### POSTGRES_HOST

**Description:** Specifies the database DNS name / IP-address for postgresql / timescaleDB 

**Type:** string

**Possible values:** all DNS names or IP 

**Example value:**  united-manufacturing-hub

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

### FACTORYINSIGHT_USER

**Description:** Specifies the admin user for the REST API 

**Type:** string

**Possible values:** all

**Example value:**  jeremy

### FACTORYINSIGHT_PASSWORD

**Description:** Specifies the password for the admin user for the REST API 

**Type:** string

**Possible values:** all

**Example value:**  changeme

### VERSION

**Description:** The version of the API used (currently not used yet) 

**Type:** int

**Possible values:** all

**Example value:**  1

### DEBUG_ENABLED

**Description:** Enables debug logging 

**Type:** string

**Possible values:** true,false

**Example value:**  false

### REDIS_URI

**Description:** URI for accessing redis sentinel  

**Type:** string

**Possible values:** All valids URIs

**Example value:** united-manufacturing-hub-redis-node-0.united-manufacturing-hub-redis-headless:26379

### REDIS_URI2

**Description:** Backup URI for accessing redis sentinel  

**Type:** string

**Possible values:** All valids URIs

**Example value:** united-manufacturing-hub-redis-node-1.united-manufacturing-hub-redis-headless:26379

### REDIS_URI3

**Description:** Backup URI for accessing redis sentinel  

**Type:** string

**Possible values:** All valids URIs

**Example value:** united-manufacturing-hub-redis-node-2.united-manufacturing-hub-redis-headless:26379

### REDIS_PASSWORD

**Description:** Password for accessing redis sentinel  

**Type:** string

**Possible values:** all 

**Example value:** changeme 


## REST API

{{< swaggerui src="/openapi/factoryinsight.yml" >}}

