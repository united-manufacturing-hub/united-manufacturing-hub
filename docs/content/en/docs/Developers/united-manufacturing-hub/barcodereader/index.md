---
title: "barcodereader"
linkTitle: "barcodereader"
date: 2021-03-15
description: >
  This is the documentation for the container barcodereader.
aliases:
  - /docs/Developers/factorycube-edge/barcodereader
  - /docs/developers/factorycube-edge/barcodereader
---


## Getting started

Here is a quick tutorial on how to start up a basic configuration / a basic docker-compose stack, so that you can develop.

Go to the root folder of the project and execute the following command:

```bash
sudo docker build -f deployment/barcodereader/Dockerfile -t barcodereader:latest . && sudo docker run --privileged -e "DEBUG_ENABLED=True" -v '/dev:/dev' barcodereader:latest 
```

All connected devices will be shown, the used device is marked with "Found xyz". After every scan the MQTT message will be printed.

## Environment variables

This chapter explains all used environment variables.

### DEBUG_ENABLED

**Description:** Deactivates MQTT and only prints the barcodes to stdout

**Type:** bool

**Possible values:** true, false

**Example value:** true

### CUSTOM_USB_NAME

**Description:** If your barcodereader is not in the supported list of devices, you must specify the name of the USB device here

**Type:** string

**Possible values:** all

**Example value:** Datalogic ADC, Inc. Handheld Barcode Scanner

### MQTT_CLIENT_ID

**Description:** The MQTT client id to connect with the MQTT broker

**Type:** string

**Possible values:** all

**Example value:** weaving_barcodereader

### BROKER_URL

**Description:** The MQTT broker URL

**Type:** string

**Possible values:** IP, DNS name

**Example value:** ia_mosquitto

**Example value 2:** localhost

### BROKER_PORT

**Description:** The MQTT broker port. Only unencrypted ports are allowed here (default: 1883)

**Type:** integer

**Possible values:** all

**Example value:** 1883

### CUSTOMER_ID

**Description:** The customer ID, which is used for the topic structure

**Type:** string

**Possible values:** all

**Example value:** dccaachen

### LOCATION

**Description:** The location, which is used for the topic structure

**Type:** string

**Possible values:** all

**Example value:** aachen

### MACHINE_ID

**Description:** The machine ID, which is used for the topic structure

**Type:** string

**Possible values:** all

**Example value:** weaving_machine_2
