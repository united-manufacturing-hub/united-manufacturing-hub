---
title: "grafana-auth-proxy"
linktitle: "grafana-auth-proxy"
date: 2021-27-07
description: >
# United Manufacturing Hub - Grafana Auth Proxy
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
Access-Control-Allow-Headers: *
Access-Control-Allow-Origin: *
```

{{< swaggerui src="/openapi/grafana-proxy.yml" >}}

