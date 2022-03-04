---
title: "sensorconnect"
linkTitle: "sensorconnect"
description: >
  This docker container automatically detects ifm gateways in the specified network and reads their sensor values in the highest possible data frequency. The MQTT output is specified in [the MQTT documentation](/docs/concepts/mqtt/)
aliases:
  - /docs/Developers/factorycube-edge/sensorconnect
  - /docs/developers/factorycube-edge/sensorconnect
---
## Sensorconnect overview
Sensorconnect provides plug-and-play access to [IO-Link](https://io-link.com/en/) sensors connected to [ifm gateways](https://www.ifm.com/us/en/category/245_010_010). A typical setup has multiple sensors (connected via IO-Link) to ifm gateways on the production shopfloor. Those gateways are connected via LAN to your server infrastructure. The sensorconnect microservice constantly monitors a given IP range for gateways. Once a gateway is found, it automatically starts requesting, receiving and processing sensordata in short intervals. The received data is preprocessed based on a database including thousands of sensor definitions. And because we strongly believe in open industry standards, Sensorconnect brings the data via MQTT or Kafka to your preferred software solutions, for example, features of the United Manufacturing Hub or the cloud.

## Which problems is Sensorconnect solving
Let's take a step back and think about, why we need a special microservice:
1. The gateways are providing a rest api to request sensordata. Meaning as long as we dont have a process to request the data, nothing will happen. 
2. Constantly requesting and processing data with high robustness and reliability can get difficult in setups with a large number of sensors.
3. Even if we use for example node-red flows ([flow from node-red](https://flows.nodered.org/node/sense-ifm-iolink), [flow from ifm](https://www.ifm.com/na/en/shared/technologies/io-link/system-integration/iiot-integration))to automatically request data from the rest api of the ifm gateways, the information is most of the times cryptic without proper interpretation. 
4. Device manufacturers will provide one IODD file (IO Device Description), for every sensor and actuator they produce. Those contain information to correctly interpret data from the devices. They are in XML-format and available at the [IODDfinder website](https://io-link.com/en/IODDfinder/IODDfinder.php). But automatic IODD interpretation is relatively complex and manually using IODD files is not economically feasible.

## Installation
### Production setup
Sensorconnect comes directly with the united manufacturing hub - no additional installation steps required. We allow the user to customize sensorconnect by changing environment variables. All possible environment variables you can use to customize sensorconnect and how you change them is described at the [environment-variables website](/docs/Developers/united-manufacturing-hub/environment-variables/). Sensorconnect is by default enabled in the United Manufacturing Hub. To set your preferred serialnumber and choose the ip range to look for gateways, you can configure either the values.yaml directly or use our managment SaaS tool. 

### Development setup
Here is a quick tutorial on how to start up a basic configuration / a basic docker-compose stack, so that you can develop.

1. execute `docker-compose -f ./deployment/sensorconnect/docker-compose.yaml up -d --build`


## Underlying Functionality
Sensorconnect downloads relevant IODD files automatically after installation from the [IODDfinder website](https://io-link.com/en/IODDfinder/IODDfinder.php). If an unknown sensor is connected later, sensorconnect will automatically download the file. We will also provide a folder to manually deposit IODD-files, if the automatic download doesn't work (e.g. no internet connection).

### Rest API post requests from sensorconnect to the gateways
Sensorconnect scans the ip range for new ifm gateways (used to connect the IO-Link devices to). To do that, sensorconnect iterates through all the possible ipaddresses in the specified ip-Address Range ("http://"+url, `payload`, timeout=0.1). It stores the ip-Adresses, with the productcodes (*the types of the gateways*) and the individual serialnumbers.

**Scanning with following `payload`: (information sent during a POST request to the ifm gateways)**
```JSON
{
  "code":"request",
  "cid":-1, // The cid (Client ID) can be chosen.
  "adr":"/getdatamulti",
  "data":{
    "datatosend":[
      "/deviceinfo/serialnumber/","/deviceinfo/productcode/"
      ]
    }
}
```
**Example answer from gateway:**
```JSON
{
    "cid": 24,
    "data": {
        "/deviceinfo/serialnumber/": {
            "code": 200,
            "data": "000201610192"
        },
        "/deviceinfo/productcode/": {
            "code": 200,
            "data": "AL1350"
        }
    },
    "code": 200
}
```
All port modes of the connected gateways are requested. Depending on the productcode, we can determine the total number of ports on the gateway and iterate through them.

**Requesting port modes with following payload: (information sent during a POST request to the ifm gateways)**
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
**Example answer from gateway:**
```JSON
{
    "cid": -1,
    "data": {
        "/iolinkmaster/port[1]/mode": {
            "code": 200,
            "data": 3
        },
        "/iolinkmaster/port[2]/mode": {
            "code": 200,
            "data": 3
        }
    }
}
```
If the mode == 1: port_mode = "DI" (Digital Input)
If the mode == 2: port_mode = "DO" (Digital output)
If the mode == 3: port_mode = "IO_Link"

All values of accessible ports are requested as fast as possible (ifm gateways are by far the bottleneck in comparison to the networking).
**Requesting IO_Link port values with following payload: (information sent during a POST request to the ifm gateways)**
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
**Example answer from gateway:**
```JSON
{
    "cid": -1,
    "data": {
        "/iolinkmaster/port[1]/iolinkdevice/deviceid": {
            "code": 200,
            "data": 278531
        },
        "/iolinkmaster/port[1]/iolinkdevice/pdin": {
            "code": 200,
            "data": "0101" // This string contains the actual data from the sensor. In this example it is the data of a buttonbar. The value changes when one button is pressed. Interpreting this value automatically relies heavily on the IODD file of the specific sensor (information about which partition of the string holds what value (type, value name etc.)).
        },
        "/iolinkmaster/port[1]/iolinkdevice/vendorid": {
            "code": 200,
            "data": 42
        },
        "/iolinkmaster/port[1]/pin2in": {
            "code": 200,
            "data": 0
        }
    },
    "code": 200
}
```

### Data interpretation (key: pdin) with IODD files
Based on the vendorid and deviceid (extracted out of the received data from the ifm-gateway), sensorconnect looks in the persistant storage if an iodd file is already on the device. If there is no iodd file stored for the sensor it tries to download a file online and save it. If this is also not possible, sensorconnect doesn't preprocess the pdin data entry and sends it as is via mqtt or kafka to the broker.

The iodd files are xml files, often with multiple thousand lines. Especially relevant areas are (for our use-case):
1. IODevice/DocumentInfo/version: version des IODD files
2. IODevice/ProfileBody/DeviceIdentity: deviceid, vendorid etc.
3. IODevice/DeviceFunction/ProcessDataCollection/ProcessData/ProcessDataIn: information about the incoming data structure. Depending on used datatypes

Now sensorconnect converts the data and sends it (as a JSON) via MQTT to the MQTT broker or via Kafka to the Kafka broker. The format of those messages coming from sensorconnect is described in detail and with examples on the [UMH Datamodel website](/docs/concepts/mqtt/).