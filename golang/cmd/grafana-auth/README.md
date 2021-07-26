# grafana-auth proxy
This program proxies requests to backend services,
if the requesting user is logged into grafana and part of the organization he requests.

## Usage

1) Set .env variables
    - FACTORYINSIGHT_KEY
    - FACTORYINSIGHT_USER
    - JAEGER_HOST
    - JAEGER_PORT

2) Run program
3) Use provided [API documentation](grafana-proxy.yml) to view it's REST API
