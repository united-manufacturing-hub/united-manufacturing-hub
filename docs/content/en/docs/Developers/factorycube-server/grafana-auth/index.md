---
title: "grafana-proxy"
linktitle: "grafana-proxy"
date: 2021-07-09
description: >
# United Manufacturing Hub - Grafana Proxy
---

This program proxies requests to backend services,
if the requesting user is logged into grafana and part of the organization he requests.

## Usage

1) Set .env variables
    - FACTORYINPUT_KEY
    - FACTORYINPUT_USER
    - FACTORYINPUT_BASE_URL
    - JAEGER_HOST
    - JAEGER_PORT

2) Run program

    - Either using ```go run github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/grafana-auth```
    - Or using the Dockerfile
        - Open an terminal inside ```united-manufacturing-hub/deployment/grafana-auth```
        - Run ```docker build -f ./Dockerfile ../..```

## Notes
Grafana-Auth accepts all CORS requests, by setting the following headers:
```
Access-Control-Allow-Headers: content-type, Authorization
Access-Control-Allow-Origin: $(REQUESTING_ORIRING)
Access-Control-Allow-Credentials: true
Access-Control-Allow-Methods: *
```

{{< swaggerui src="/openapi/grafana-proxy.yml" >}}

