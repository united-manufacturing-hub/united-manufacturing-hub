---
title: "cameraconnect"
linkTitle: "cameraconnect"
description: >
  This docker container automatically detects cameras in the network and makes them accessible via MQTT. The MQTT output is specified in [the MQTT documentation](/docs/concepts/mqtt/)
aliases:
  - /docs/Developers/factorycube-edge/cameraconnect
  - /docs/developers/factorycube-edge/cameraconnect
---

**This microservice is still in development and is not considered stable for production use.**

## Getting started

### Using the Helm chart

By default cameraconnect will be deactivated in united-manufacturing-hub. First, you need to enable it in the united-manufacturing-hub values. Then you need to create a folder on the node in `/home/rancher/gentl_producer` and move your genTL producer files (*.cti) and all required libaries into that folder. Then apply your settings to the Helm chart with `helm upgrade`.

Another idea for changing it:
`helm install ....... --set 'cameraconnect.enabled=true'`

Or overwroite it in the `development_values.yaml`

Furthermore, you need to adjust the MQTT_HOST to the externally exposed MQTT IP (e.g., the IP of your node). Usually you can use the Kubernetes internal DNS. But cameraconnect needs to be in hostMode = true and hterefore you need to access from external. 

### Development setup

Here is a quick tutorial on how to start up a basic configuration / a basic docker-compose stack, so that you can develop.

1. Specify the environment variables, e.g. in a .env file in the main folder or directly in the docker-compose
2. execute `sudo docker-compose -f ./deployment/cameraconnect/docker-compose.yaml up -d --build`

## Environment variables

This chapter explains all used environment variables.

### CUBE_TRANSMITTERID

Description: The unique transmitter id. This will be used for the creation of the MQTT topic. ia/raw/TRANSMITTERID/...

Type: string

Possible values: all

Example value: 2021-0156

### MQTT_HOST

**Description:** The MQTT broker URL

**Type:** string

**Possible values:** IP, DNS name

**Example value:** ia_mosquitto

**Example value 2:** localhost

### MQTT_PORT

**Description:** The MQTT broker port. Only unencrypted ports are allowed here (default: 1883)

**Type:** integer

**Possible values:** all

**Example value:** 1883

### TRIGGER

**Description:** Defines the option of how the camera is triggered. Either via MQTT or via a continuous time trigger. <br>
In production the camera should be triggered via MQTT. The continuous time trigger is just convenient for debugging.<br>
If MQTT is selected, the camera will be triggered by any message which arrives via its subscribed MQTT topic.<br>
However, if the arriving MQTT message contains a UNIX timestamp in milliseconds with the key "timestamp_ms", <br>
the camera will be triggered at that exact timestamp.

**Type:** string

**Possible values:** MQTT, Continuous

**Example value:** MQTT

### ACQUISITION_DELAY

**Description:** Timeconstant in seconds which delays the image acquisition after the camera has been triggered.<br>
This is mostly used, if the camera is triggered with a UNIX timestamp (see variable TRIGGER), to make sure, that the <br>
camera is triggered, even if the UNIX timestamps lies in the past. This could be caused by network latencies.


**Type:** float

**Possible values:** all

**Example value:** 0.7

### CYCLE_TIME

**Description:** Only relevant if the trigger is set to "Continuous". Cycle time gives the time period which defines<br> the frequency in which the camera is triggered.
<br>For example: a value of 0.5 would result in a trigger frequency of 2 images per second.

**Type:** float

**Possible values:** all

**Example value:** 1.5

### CAMERA_INTERFACE

**Description:** Defines which camera interface is used. Currently only cameras of the GenICam standard are supported. <br>
However, for development of testing you can also use the DummyCam, which simulates a camera and sends a static image via MQTT, <br>when triggered.

**Type:** String

**Possible values:** GenICam, DummyCam

**Example value:** GenICam

### EXPOSURE_TIME

**Description:** Defines the exposure time for the selected camera. You should adjust this to your local environment to<br> achieve optimal images.

**Type:** int

**Possible values:** Depends on camera interface. Values between 1 and 80,000 are eligible for most cameras.

**Example value:** 1000

### EXPOSURE_AUTO

**Description:**  Determines if camera automatically adjusts the exposure time. Your settings will only be executed if <br>
the camera supports this. You do not have to check if the camera supports this.

**Type:** String

**Possible values:** <br>"Off" -  No automatic adjustment <br>
"Once" - Adjusted once<br>
"Continuous" - Continuous adjustment (not recommended, Attention: This could have a big impact on the frame rate of your camera)

**Example value:** Off

### PIXEL_FORMAT

**Description:**  Sets the pixel format which will be used for image acquisition. This module allows you to acquire <br>
images in monochrome pixel formats(use: "Mono8") and RGB/BRG color pixel formats (use:"RGB8Packed" or "BGR8Packed")

**Type:** String

**Possible values:** Mono8, RGB8Packed, BGR8Packed

**Example value:** Mono8

### IMAGE_WIDTH

**Description:**  Defines the horizontal width of the images acquired. If the width values surpasses the maximum <br>capability of the camera
the maximum value is set automatically.

**Type:** int

**Possible values:** all except 0

**Example value:** 1000

### IMAGE_HEIGHT

**Description:**  Defines the vertical height of the images acquired. If the height value surpasses the maximum <br>capability of the camera
the maximum value is set automatically.

**Type:** int

**Possible values:** all except 0

**Example value:** 1000

### IMAGE_CHANNELS

**Description:**  Number of channels (bytes per pixel) that are used in the array (third dimension of the image data <br>
array).You do not have to set this value. If None, the best number of channels for your set pixel format will be used.

**Type:** int

**Possible values:** 1 or 3

**Example value:** 1

### MAC_ADDRESS

**Description:**  Defines which camera is accessed by the container. One container can use only one camera. <br>
The MAC address can be found on the backside of the camera.<br>
The input is not case sensitive. Please follow the example format below.

**Type:** String

**Possible values:** all

**Example value:** 0030532B879C

### LOGGING_LEVEL

**Description:**  Defines which logging level is used. Mostly relevant for developers. Use WARNING or ERROR in production.

**Type:** String

**Possible values:** DEBUG, INFO, WARNING, ERROR, CRITICAL

**Example value:** DEBUG


## Credits

Based on the Bachelor Thesis from [Patrick Kunz](../../../publications/plug-and-play-visual-quality-inspection).
