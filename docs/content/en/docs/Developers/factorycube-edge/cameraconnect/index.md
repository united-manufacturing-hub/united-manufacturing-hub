---
title: "cameraconnect"
linkTitle: "cameraconnect"
description: >
  This docker container 
---

## Getting started

Here is a quick tutorial on how to start up a basic configuration / a basic docker-compose stack, so that you can develop.


TODO 

1. Specify the environment variables, e.g. in a .env file in the main folder or directly in the docker-compose
2. execute `sudo docker-compose -f ./deployment/sensorconnect/docker-compose.yaml up -d --build`

Upload GenICam GenTL producer using:
`kubectl create configmap cameraconnect-gentlproducer --from-file <ZIP FILEi CONTAINING CTI FILES WITHTHE NAME FILES.ZIP>`

## Environment variables

This chapter explains all used environment variables.

TODO 
### TRANSMITTERID

Description: The unique transmitter id. This will be used for the creation of the MQTT topic. ia/raw/TRANSMITTERID/...

Type: string

Possible values: all

Example value: 2021-0156

