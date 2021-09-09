---
title: "grafana-proxy"
linktitle: "grafana-proxy"
description: > Documentation of grafana-proxy
---

{{% notice warning %}}
This microservice is still in development and is not considered stable for production use.
{{% /notice %}}

## Getting started

This program proxies requests to backend services, if the requesting user is logged into grafana and part of the organization he requests.

Either using `go run github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/grafana-auth`
Or using the Dockerfile
    - Open an terminal inside `united-manufacturing-hub/deployment/grafana-auth`
    - Run `docker build -f ./Dockerfile ../..`

## Environment variables

This chapter explains all used environment variables.

### FACTORYINPUT_USER

**Description:** Specifies the user for the REST API 

**Type:** string

**Possible values:** all

**Example value:**  jeremy

### FACTORYINPUT_KEY

**Description:** Specifies the password for the user for the REST API 

**Type:** string

**Possible values:** all

**Example value:**  changeme

### FACTORYINPUT_BASE_URL

**Description:** Specifies the DNS name / IP-address to connect to factoryinput

**Type:** string

**Possible values:** all DNS names or IP 

**Example value:**  http://factorycube-server-factoryinput-service

### FACTORYINSIGHT_BASE_URL

**Description:** Specifies the DNS name / IP-address to connect to factoryinsight

**Type:** string

**Possible values:** all DNS names or IP

**Example value:**  http://factorycube-server-factoryinsight-service

### LOGGING_LEVEL

**Description:** Optional variable, if set to "DEVELOPMENT", it will switch to debug logging

**Type:** string

**Possible values:** Any

**Example value:**  DEVELOPMENT

### JAEGER_HOST

**Description:** Optional variable, Jaeger tracing host

**Type:** string

**Possible values:** all DNS names or IP

**Example value:**  http://jaeger.localhost

### JAEGER_PORT

**Description:** Optional variable, Port for Jaeger tracing

**Type:** string

**Possible values:** 0-65535

**Example value:**  9411

### DISABLE_JAEGER

**Description:** Optional variable, disables Jaeger if set to 1 or true

**Type:** string

**Possible values:** Any

**Example value:**  1





## Notes
Grafana-Proxy accepts all cors requests.
For authenticated requests, you *must* send your Origin, else CORS will fail
```
Access-Control-Allow-Headers: content-type, Authorization
Access-Control-Allow-Origin: $(REQUESTING_ORIRING) or *
Access-Control-Allow-Credentials: true
Access-Control-Allow-Methods: *
```

{{< swaggerui src="/openapi/grafana-proxy.yml" >}}

