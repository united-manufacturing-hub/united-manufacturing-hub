---
title: "1. Setting up factorycube-edge"
linkTitle: "1. Setting up factorycube-edge"
weight: 1
description: >
  This section explains how factorycube-edge can be installed on the edge devices to extract and process data 
---

## Method 1: manual installation using k3os 
Recommended for small deployments 

### Prerequisites

- k3os installed on a bootable USB-stick (you can get it here: https://github.com/rancher/k3os/releases/)
- a edge device

### Steps

1. Install k3os and helm on your edge device
2. Copy the helm chart to the folder `/home/rancher/factorycube-edge` (you can use other folders as well, but please adjust the folder name in the following code snippets)
3. Configure `factorycube-edge` by creating a values.yaml file in the folder `/home/rancher/configs/factorycube-edge-helm-values.yaml`, which you use to override values. Required: `mqttBridgeURL`, `mqttBridgeTopic`, `sensorconnect.iprange`, `mqttBridgeCACert`, `mqttBridgeCert`, `mqttBridgePrivkey`
4. Execute `helm install factorycube-edge /home/rancher/factorycube-edge --values "/home/rancher/configs/factorycube-edge-helm-values.yaml" --set serialNumber=$(hostname) --kubeconfig /etc/rancher/k3s/k3s.yaml` (change kubeconfig and serialNumber accordingly)

Please always use `factorycube-edge` as name for the release. Unfortunately, there are still some bugs with other names.

### Usage

You can now log into the device using the SSH keys you specified in the config file. You can get the kubeconfig with `cat /etc/rancher/k3s/k3s.yaml`.

## Method 2: using factorycube-edge-deployment
Recommended for large deployments. Enterprise only.

### Prerequisites

- k3os installed on a bootable USB-stick (you can get it here: https://github.com/rancher/k3os/releases/)
- a edge device
- Download and setup factorycube-deployment on a server in your local network acording to the README
- Created all required SSL certificates and pasted them into the config file
- Specified SSH keys and other information in the config file

### During installation of k3os

Boot from the USB stick and select the drive. Now you can use the following cloud-init script during the installation of k3os: `http://YOUR_IP/configs/SERIAL_NUMBER.yaml`

Press enter and remove USB-stick.

### Usage

See also Method 1

## Method 3: using certified devices
Recommended for large deployments. 

<img src="/images/products/factorycube.png" style="height: 50px !important"> <img src="/images/products/cubi.png" style="height: 50px !important">

We offer certified devices either for the retrofit (factorycube) or for connecting with the PLC (cubi). Take a look at our [website](https://united-manufacturing-hub.com) for more information on our certified devices.

### Usage

See also Method 1

## Method 4: using Ubuntu instead of k3os
Recommended for development or learning modules. Not recommended for production.

### Prerequisites

TODO

### Steps

TODO
