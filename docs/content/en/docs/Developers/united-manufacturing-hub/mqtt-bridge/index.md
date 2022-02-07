---
title: "mqtt-bridge"
linkTitle: "mqtt-bridge"
description: "This tool acts as an MQTT bridge to handle bad internet connections."
aliases:
  - /docs/Developers/factorycube-edge/mqtt-bridge
  - /docs/developers/factorycube-edge/mqtt-bridge
---

## Getting started

Here is a quick tutorial on how to start up a basic configuration / a basic docker-compose stack, so that you can develop.

Here is a quick tutorial on how to start up a basic configuration / a basic docker-compose stack, so that you can develop.

1. Specify the environment variables, e.g. in a .env file in the main folder or directly in the docker-compose
2. execute `sudo docker-compose -f ./deployment/mqtt-bridge/docker-compose.yaml up -d --build`

## Environment variables

This chapter explains all used environment variables.

### REMOTE_CERTIFICATE_NAME

Description: the certificate name / client id

Type: string

Possible values: all

Example value: 2021-0156

### REMOTE_BROKER_URL 

Description: the URL to the broker. Can be prepended either with ssl:// or mqtt:// or needs to specify the port afterwards with :1883 

Type: string

Possible values: all

Example value: ssl://mqtt.app.industrial-analytics.net

### REMOTE_SUB_TOPIC 

Description: the remote topic that should be subscribed. The bridge will automatically append a /# to the string mentioned here

Type: string

Possible values: all

Example value: ia/ia 

### REMOTE_PUB_TOPIC 

Description: the remote topic prefix where messages from the remote broker should be send to.

Type: string

Possible values: all

Example value: ia/ia 

### REMOTE_BROKER_SSL_ENABLED

Description: should SSL be enabled and certificates be used for connection? 

Type: bool 

Possible values: true or false 

Example value: true 

### LOCAL_CERTIFICATE_NAME

Description: the certificate name / client id

Type: string

Possible values: all

Example value: 2021-0156

### LOCAL_BROKER_URL 

Description: the URL to the broker. Can be prepended either with ssl:// or mqtt:// or needs to specify the port afterwards with :1883 

Type: string

Possible values: all

Example value: ssl://mqtt.app.industrial-analytics.net

### LOCAL_SUB_TOPIC 

Description: the remote topic that should be subscribed. The bridge will automatically append a /# to the string mentioned here

Type: string

Possible values: all

Example value: ia/ia 

### LOCAL_PUB_TOPIC 

Description: the remote topic prefix where messages from the remote broker should be send to.

Type: string

Possible values: all

Example value: ia/ia 

### LOCAL_BROKER_SSL_ENABLED

Description: should SSL be enabled and certificates be used for connection? 

Type: bool 

Possible values: true or false 

Example value: true 

### BRIDGE_ONE_WAY

Description: DO NOT SET TO FALSE OR THIS MIGHT CAUSE AN ENDLESS LOOP! NEEDS TO BE FIXED BY SWITCHING TO MQTTV5 AND USING NO_LOCAL OPTION WHILE SUBSCRIBING. If true it sends the messages only from local broker to remote broker (not the other way around) 

Type: bool 

Possible values: true or false 

Example value: true 

## Important note regarding topics

The bridge will append /# to LOCAL_SUB_TOPIC and subscribe to it. All messages will then be send to the remote broker. The topic on the remote broker is defined by:
1. First stripping LOCAL_SUB_TOPIC from the topic
2. and then replacing it with REMOTE_PUB_TOPIC
