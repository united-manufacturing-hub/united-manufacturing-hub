---
title: "sensorconnect"
linkTitle: "sensorconnect"
description: >
  This docker container automatically detects ifm gateways in the specified network and reads their sensor values in the highest possible data frequency. The MQTT output is specified in [the MQTT documentation](/docs/concepts/mqtt/)
aliases:
  - /docs/Developers/factorycube-edge/sensorconnect
  - /docs/developers/factorycube-edge/sensorconnect
---
## Sensorconnect
Sensorconnect provides plug-and-play access to IO-Link sensors connected to IFM gateways. A typical setup has multiple sensors connected via IO-Link to ifm gateways on the shopfloor. Those gateways are connected to LAN. The sensorconnect microservice constantly monitors a given IP range for gateways. Once found, it will request sensordata in short intervals. The received data is preprocessed based on a database including thousands of sensor definitions. And because we strongly believe in open industry standards, Sensorconnect brings the data via MQTT or Kafka to your preferred software solutions, for example, features of the United Manufacturing Hub.


## Getting started
### Production setup
Sensorconnect is by default implemented in the United Manufacturing Hub. You can use our Management SaaS tool to enable it, set your preffered transmitter id and choose the ip range to look for gateways. 
### Development setup
Here is a quick tutorial on how to start up a basic configuration / a basic docker-compose stack, so that you can develop.

1. execute `docker-compose -f ./deployment/sensorconnect/docker-compose.yaml up -d --build`



## Underlying Functionality

{{< imgproc sensorconnectOverviewImage Fit "2026x1211" >}}{{< /imgproc >}}

Sensorconnect is based on IO-Link. Device manufacturers will provide one IODD file (IO Device Description), for every sensor and actuator they produce.
Those contain information to correctly interpret data from the devices. They are in XML-format. Sensorconnect will download relevant IODD files automatically after installation from the IODDfinder website (https://io-link.com/en/IODDfinder/IODDfinder.php). We will also provide a folder to manually deposit IODD-files, if the automatic download doesn't work.

Sensorconnect will scan the ip range for new ifm gateways (used to connect the IO-Link devices to). To do that, sensorconnect iterates through all the possible ipaddresses in the specified ip-Address Range ("http://"+url, `payload`, timeout=0.1). It stores the ip-Adresses, with the productcodes (*the types of the devices*) and the individual serialnumbers.

**Scanning with following `payload`: (information sent during a POST request to the ifm gateways)**
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

Sensorconnect receives 


Based on the VendorIdentifier and DeviceIdentifier (specified in the received data from the ifm-gateway), sensorconnect can look up relevant information from the IODD file to interpret the data.

Now sensorconnect converts the data and sends it (as a JSON) via MQTT to the MQTT broker. The format from sensorconnect is described in detail and with examples on the [UMH Datamodel website](/docs/concepts/mqtt/).