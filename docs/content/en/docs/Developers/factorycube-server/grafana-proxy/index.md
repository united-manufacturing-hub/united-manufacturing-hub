---
title: "grafana-proxy"
linktitle: "grafana-proxy"
description: > Documentation of grafana-proxy
---

This program proxies requests to backend services, if the requesting user is logged into grafana and part of the organization he requests.

## Getting started

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



## Notes
Grafana-Auth accepts all CORS requests, by setting the following headers:
```
Access-Control-Allow-Headers: content-type, Authorization
Access-Control-Allow-Origin: $(REQUESTING_ORIRING)
Access-Control-Allow-Credentials: true
Access-Control-Allow-Methods: *
```

{{< swaggerui src="/openapi/grafana-proxy.yml" >}}

