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

1. Install k3os on your edge device
2. Install Helm on your edge device
```
export VERIFY_CHECKSUM=false 
curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 
chmod 700 get_helm.sh && ./get_helm.sh
```
3. Copy the content of `united-manufacturing-hub/deployment/factorycube-core/` of our GitHub repo repo into the folder of your edge device `/home/rancher/factorycube-core` (you can use other folders as well, but please adjust the folder name in the following code snippets)
4. Configure `factorycube-core` by creating a `factorycube-core-helm-values.yaml` file in the folder `/home/rancher/configs/factorycube-core-helm-values.yaml`, which you use to override values. Required: `mqttBridgeURL`, `mqttBridgeTopic`, `sensorconnect.iprange`, `mqttBridgeCACert`, `mqttBridgeCert`, `mqttBridgePrivkey`

Example for factorycube-core-helm-values.yaml:
```
mqttBridgeURL: "ssl://mqtt.umh.app:8883"
mqttBridgeTopic: "ia/ia"
sensorconnect:
  iprange: "172.16.1.0/24"
mqttBridgeCACert: |
  ENTER CERT HERE
mqttBridgeCert: |
  ENTER CERT HERE
mqttBridgePrivkey: |
  ENTER CERT HERE
```

5. Execute `helm install factorycube-core /home/rancher/factorycube-core --values "/home/rancher/configs/factorycube-core-helm-values.yaml" --set serialNumber=$(hostname) --kubeconfig /etc/rancher/k3s/k3s.yaml` (change kubeconfig and serialNumber accordingly)

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
