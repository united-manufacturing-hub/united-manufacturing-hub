---
title: "factoryinput"
linktitle: "factoryinput"
date: 2021-07-09
description: >
# United Manufacturing Hub - Factoryinput
---

This program provides an REST endpoint, to send MQTT messages via HTTP requests

# Usage
1) Setup .env variables
   - Required
     - FACTORYINPUT_USER
       - Administrative user for the REST API
     - FACTORYINPUT_PASSWORD
       - Password of the admin user
     - VERSION
       - Version of the REST API to use
         - Currently, only '1' is a valid value
     - CERTIFICATE_NAME
       - Certificate for MQTT authorization or NO_CERT
     - MY_POD_NAME
       - MQTT Client Id
     - BROKER_URL
       - URL of MQTT Broker

   - Optional
      - CUSTOMER_NAME_{Number}
         - Name of authorized customer for the REST API
      - CUSTOMER_PASSWORD_{Number}
         - Password of authorized customer
      - LOGGING_LEVEL
         - DEVELOPMENT
         - Any other value will be parsed as production (including not set)
      - JAEGER_HOST
        - Host for Jaeger tracing
      - JAEGER_PORT
        - Port for Jaeger tracing

2) Run the program
   - Either using ```go run github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinput```
   - Or using the Dockerfile
      - Open a terminal inside ```united-manufacturing-hub```
      - Run ```docker build -f .\deployment\mqtt-to-postgresql\Dockerfile .```
      - Look for the image SHA 
        - Example: ```=> => writing image sha256:11e4e669d6581df4cb424d825889cf8989ae35a059c50bd56572e2f90dd6f2bc```
      - ```docker run SHA```
        - ```docker run 11e4e669d6581df4cb424d825889cf8989ae35a059c50bd56572e2f90dd6f2bc -e VERSION=1 ....```



# Rest API 
{{< swaggerui src="/openapi/factoryinput.yml" >}}
