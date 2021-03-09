# Installing the core stack

## Contents

- [Installing the core stack](#installing-the-core-stack)
  - [Contents](#contents)
  - [Method 1: using factorycube-core-deployment](#method-1-using-factorycube-core-deployment)
  - [Method 2: doing it on your own](#method-2-doing-it-on-your-own)

## Introduction

This document describes how the core stack can be used and how it works.

## Quick start

There are two methods to get started with the core stack. Both methods require that you have a edge device and a k3os instalation medium available.

### Method 1: using factorycube-core-deployment

The most scalable method is to use `factorycube-core-deployment`. This allows you to host the entire configuration files at a webserver, which you can then fetch during the k3os installation using the cloud-init script. 

See also [here](factorycube-core-deployment.md).

### Method 2: doing it on your own

If you do not want to use the deployment script you can install it on your own.

1. Install k3os and helm on your edge device
2. Copy the helm chart to the folder `/home/rancher/factorycube-core` (you can use other folders as well, but please adjust the folder name in the following code snippets)
3. Configure `factorycube-core` by creating a values.yaml file in the folder `/home/rancher/configs/factorycube-core-helm-values.yaml`, which you use to override values. Required: `mqttBridgeURL`, `mqttBridgeTopic`, `sensorconnect.iprange`, `mqttBridgeCACert`, `mqttBridgeCert`, `mqttBridgePrivkey`
4. Execute `helm install factorycube-core /home/rancher/factorycube-core --values "/home/rancher/configs/factorycube-core-helm-values.yaml" --set serialNumber=$(hostname) --kubeconfig /etc/rancher/k3s/k3s.yaml` (change kubeconfig and serialNumber accordingly)


## Architecture

The factorycube-core stack consists out of one helm chart which is then deployed in a local Kubernetes cluster on your edge device.

It consists out of:

- the edge version of the EMQ X MQTT Broker
- nodered
- mqtt-bridge (to send data to the cloud from the local broker)

All important values can be changed in the values.yaml. See also [helm documention](https://helm.sh/docs/) for more information.
