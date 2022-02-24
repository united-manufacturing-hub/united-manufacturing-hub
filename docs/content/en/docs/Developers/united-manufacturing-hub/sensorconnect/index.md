---
title: "sensorconnect"
linkTitle: "sensorconnect"
description: >
  This docker container automatically detects ifm gateways in the specified network and reads their sensor values in the highest possible data frequency. The MQTT output is specified in [the MQTT documentation](/docs/concepts/mqtt/)
aliases:
  - /docs/Developers/factorycube-edge/sensorconnect
  - /docs/developers/factorycube-edge/sensorconnect
---

## Getting started

Here is a quick tutorial on how to start up a basic configuration / a basic docker-compose stack, so that you can develop.

1. execute `docker-compose -f ./deployment/sensorconnect/docker-compose.yaml up -d --build`


## Environment variables

This chapter explains all used environment variables.

### TRANSMITTERID

**Description**: The unique transmitter id. This will be used for the creation of the MQTT/Kafka topic. ia/raw/TRANSMITTERID/...

**Type**: string

**Possible values**: all

**Example value**: 2021-0156

**Example value 2**: development

### IP_RANGE

**Description:** The IP range to search for ifm gateways.

**Type:** string

**Possible values:** All subnets in [CIDR notation](https://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing)

**Example value:** 172.16.0.0/24


### IODD_FILE_PATH

**Description:** A persistent path, to save downloaded IODD files at

**Type:** string

**Possible values:** All valid paths

**Example value:** /tmp/iodd

### LOWER_POLLING_TIME_MS

**Description:** The fastest time to read values from connected sensors in milliseconds

**Type:** int

**Possible values:** all

**Example value:** 100


### UPPER_POLLING_TIME_MS

**Description:** The slowest time to read values from connected sensors in milliseconds

**Note:** To disable this feature, set this variable to the same value as LOWER_POLLING_TIME_MS

**Type:** int

**Possible values:** all

**Example value:** 100

### POLLING_SPEED_STEP_UP_MS

**Description:** The time to add to the actual tick rate in case of a failure (incremental)

**Type:** int

**Possible values:** all

**Example value:** 10

### POLLING_SPEED_STEP_DOWN_MS

**Description:** The time to subtract from actual tick rate in case of a failure (incremental)

**Type:** int

**Possible values:** all

**Example value:** 10


### SENSOR_INITIAL_POLLING_TIME_MS

**Description:** The tick speed, that sensor connect will start from. Set a bit higher than LOWER_POLLING_TIME_MS to allow sensors to recover from faults easier

**Type:** int

**Possible values:** all

**Example value:** 100

### MAX_SENSOR_ERROR_COUNT

**Description:** After this numbers on errors, the sensor will be removed until the next device scan 

**Type:** int

**Possible values:** all

**Example value:** 50


### DEVICE_FINDER_TIME_SEC

**Description:** Seconds between scanning for new devices

**Type:** int

**Possible values:** all

**Example value:** 10


### DEVICE_FINDER_TIMEOUT_SEC

**Description:** Timeout per device for scan response

**Type:** int

**Possible values:** all

**Example value:** 10


### ADDITIONAL_SLEEP_TIME_PER_ACTIVE_PORT_MS

**Description:** This adds a sleep per active port on an IO-Link master

**Type:** float

**Possible values:** all

**Example value:** 0.0


### SUB_TWENTY_MS

**Description:** Allows query times below 20MS. DO NOT activate this, if you are unsure, that your devices survive this load.

**Type:** bool

**Possible values:** 0,1

**Example value:** 0


### ADDITIONAL_SLOWDOWN_MAP

**Description:** A json map of additional slowdowns per device. Identificates can be Serialnumber, Productcode or URL

**Type:** int

**Possible values:** all

**Example value:** []
**Example value:** ```json[{"serialnumber":"000200610104","slowdown_ms":-10},{"url":"http://192.168.0.13","slowdown_ms":20},{"productcode":"AL13500","slowdown_ms":20.01}]```


#### MQTT only

### USE_MQTT

**Description:** Enables sending using MQTT

**Type:** bool

**Possible values:** true, false, 0, 1

### MQTT_BROKER_URL

**Description:** The MQTT broker URL (with port)

**Type:** string

**Possible values:** IP, DNS name

**Example value:** united-manufacturing-hub-vernemq-local-service:1883

**Example value 2:** localhost:1883

### MQTT_CERTIFICATE_NAME

**Description:** The name of the certificate to use

**Type:** string

**Possible values:** all

**Example value:** NO_NAME

#### Kafka only

### USE_KAFKA

**Description:** Enables sending using Kafka

**Type:** bool

**Possible values:** true, false, 0, 1

### MQTT_BROKER_URL

**Description:** The Kafka boostrap server url

**Type:** string

**Possible values:** IP, DNS name

**Example value:** united-manufacturing-hub-kafka:9092


## Underlying Functionality
*We are right now rewriting sensorconnect &rarr; the following paragraph might not be fully implemented yet!*

{{< imgproc sensorconnectOverviewImage Fit "2026x1211" >}}{{< /imgproc >}}

Sensorconnect is based on IO-Link. Device manufacturers will provide one IODD file (IO Device Description), for every sensor and actuator they produce.
Those contain information, e.g. necessary to correctly interpret data from the devices. They are in XML-format. Sensorconnect will try to download relevant IODD files automatically after installation from the IODDfinder (https://io-link.com/en/IODDfinder/IODDfinder.php). We will also provide a folder to manually deposit IODD-files, if the automatic download doesn't work.

Sensorconnect will scan all 10 seconds for new ifm gateways (used to connect the IO-Link devices to). To do that, sensorconnect iterates through all the possible ipaddresses in the specified ip-Address Range ("http://"+url, `payload`, timeout=0.1). It stores the ip-Adresses, with the productcodes (*the types of the devices*) and the individual serialnumbers.

**Scanning with following `payload`:**
```JSON
{
  "code":"request",
  "cid":-1,
  "adr":"/getdatamulti",
  "data":{
    "datatosend":[
      "/deviceinfo/serialnumber/","/deviceinfo/productcode/"
      ]
    }
}
```

All port modes of the connected gateways are requested every 10 seconds. Depending on the productcode, we can determine the total number of ports on the gateway and iterate through them.
**Requesting port modes with following payload:**
```JSON
{
  "code":"request",
  "cid":-1,
  "adr":"/getdatamulti",
  "data":{
    "datatosend":[
      "/iolinkmaster/port[1]/mode",
      "/iolinkmaster/port<i>/mode" //looping through all ports on gateway
      ]
    }
}
```

If the mode == 1: port_mode = "DI" (Digital Input)
If the mode == 2: port_mode = "DO" (Digital output)
If the mode == 3: port_mode = "IO_Link"

All values of accessible ports are requested as fast as possible (ifm gateways are by far the bottleneck in comparison to the networking).
**Requesting IO_Link port values with following payload:**
```JSON
{
  "code":"request",
  "cid":-1,
  "adr":"/getdatamulti",
  "data":{
    "datatosend":[
      "/iolinkmaster/port[1]/iolinkdevice/deviceid",
      "/iolinkmaster/port[1]/iolinkdevice/pdin",
      "/iolinkmaster/port[1]/iolinkdevice/vendorid",
      "/iolinkmaster/port[1]/pin2in",
      "/iolinkmaster/port[<i>]/iolinkdevice/deviceid",//looping through all connected ports on gateway
      "/iolinkmaster/port[<i>]/iolinkdevice/pdin",
      "/iolinkmaster/port[<i>]/iolinkdevice/vendorid",
      "/iolinkmaster/port[<i>]/pin2in"
      ]
  }
}
```
*TODO: Add request for DI an DO ports!!!*


Based on the VendorIdentifier and DeviceIdentifier (specified in the received data from the ifm-gateway), sensorconnect can look up relevant information from the IODD file to interpret the data.

Now sensorconnect converts the data and sends it (as a JSON) via MQTT to the MQTT broker. The format from sensorconnect is described in detail and with examples on the [UMH Datamodel website](/docs/concepts/mqtt/).