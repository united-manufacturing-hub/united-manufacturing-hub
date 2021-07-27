# grafana-auth proxy
This program proxies requests to backend services,
if the requesting user is logged into grafana and part of the organization he requests.

## Usage

1) Set .env variables
    - FACTORYINPUT_KEY
    - FACTORYINPUT_USER
    - JAEGER_HOST
    - JAEGER_PORT

2) Run program
   
   - Either using ```go run github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/grafana-auth```
   - Or using the Dockerfile
      - Open an terminal inside ```united-manufacturing-hub/deployment/grafana-auth```
      - Run ```docker build -f ./Dockerfile ../..```
   
3) Use provided [API documentation](grafana-proxy.yml) to view it's REST API
