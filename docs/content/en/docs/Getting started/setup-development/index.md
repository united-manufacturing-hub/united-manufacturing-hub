---
title: "1. Quick start"
linkTitle: "1. Quick start"
weight: 1
description: >
  This section explains how the system (edge and server) can be setup quickly on a single edge device. This is only recommended for development and testing environments.
---


## Prerequisites

- k3os installed on a bootable USB-stick (you can get it here: https://github.com/rancher/k3os/releases/). You can create a bootable USB-stick using [balenaEtcher](https://www.balena.io/etcher/)
- a laptop with SSH / SFTP client (e.g. [MobaXTerm](https://mobaxterm.mobatek.net/)) and [Lens](https://k8slens.dev/) (for accessing the Kubernetes cluster) installed
- a edge device (currently only x86 systems supported)
- keyboard, monitor, cables
- A GitHub account with a public key. If you do not know how to do it [check out this tutorial](https://gist.github.com/dmangiarelli/1a0ae107aaa5c478c51e#ssh-setup-with-putty). You can download puttygen [here](https://the.earth.li/~sgtatham/putty/latest/w64/puttygen.exe)
- network setup and internet access according to the image below

{{< imgproc development-network.png Fit "500x300" >}}{{< /imgproc >}}

## Steps

### k3OS

1. Install k3OS on your edge device using the bootable USB-stick 
2. When asked, enter your GitHub username. In the future you will access the device via SSH with your private key. After the installation the system will reboot and show after successfull startup the IP adress of the device. If no IP is shown please check your network setup (especially whether you have DHCP activated). 
3. You can now disconnect Monitor and keyboard as you will do everything else via SSH.

### General setup

1. Connect via SSH e.g. with MobaXTerm to the device and specify your private key during authentification. Please use `rancher` as username.
2. Install helm on your edge device
```bash
export VERIFY_CHECKSUM=false 
curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 
chmod 700 get_helm.sh && ./get_helm.sh
```
3. Clone or copy the content of the [united-manufacturing-hub repository on Github](https://github.com/united-manufacturing-hub/united-manufacturing-hub) into the home folder (`/home/rancher/united-manufacturing-hub`).
4. Execute `cat /etc/rancher/k3s/k3s.yaml` to retrieve the secrets to connect to your Kubernetes cluster
5. Paste the file into Lens when adding a new cluster and adjust the IP 127.0.0.1 with the IP you got at the end of the k3OS step. You should now see the cluster in Lens.
6. Create two namespaces in your Kubernetes cluster called `factorycube-edge` and `factorycube-server` by executing the following command:
```bash
kubectl create namespace factorycube-edge && kubectl create namespace factorycube-server
```

### Install factorycube-edge

Warning: in production you should use your own certificates and not the ones provided by us as this is highly insecure when exposed to the internet and any insecure network. A tutorial to setup PKI infrastructure for MQTT according to [this guide](../../tutorials/pki)

1. Go into the folder `/home/rancher/united-manufacturing-hub/deployment/factorycube-edge`
2. Create a new file called `development_values.yaml` using `touch development_values.yaml`
3. Copy the following content to that file or use the following example [`development_values.yaml`](/examples/factorycube-server/development_values.yaml). 
4. (only if you did not use example file) Copy the certificates in `deplotment/factorycube-server/developmentCertificates/pki/` and then `ca.crt`, `issued/TESTING.crt` and `issued/private/TESTING.key` into `development_values.yaml`. Additionally use as `mqttBridgeURL` `ssl://factorycube-server-vernemq-local-service.factorycube-server:8883`. 
5. Adjust `iprange` to your network IP range

Example for `development_values.yaml`:
```yaml
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

6. Execute `helm install factorycube-edge /home/rancher/united-manufacturing-hub/deployment-factorycube-edge --values "/home/rancher/united-manufacturing-hub/deployment/development_values.yaml" --set serialNumber=$(hostname) --kubeconfig /etc/rancher/k3s/k3s.yaml` (change kubeconfig and serialNumber accordingly)

### Install factorycube-server

Warning: in production this should be installed on a seperate device / in the cloud to ensure High Availability and provide automated backups. 

1. Go to the folder `deployment/factorycube-server`
2. Configure values.yaml according to your needs. For the development version you do not need to do anything. For help in configuring you can take a look into the respective documentation of the subcharts ([Grafana](https://github.com/grafana/helm-charts), [redis](https://github.com/bitnami/charts/tree/master/bitnami/redis), [timescaleDB](https://github.com/timescale/timescaledb-kubernetes/tree/master/charts/timescaledb-single), [verneMQ](https://github.com/vernemq/docker-vernemq/tree/master/helm/vernemq)) or into the documentation of the subcomponents ([factoryinsight](../../developers/factorycube-server/factoryinsight), [mqtt-to-postgresql](../../developers/factorycube-server/mqtt-to-postgresql))
3. Execute `helm install factorycube-server .` and wait. Helm will automatically install the entire stack across multiple node. It can take up to several minutes until everything is setup. 

Everything should be now successfully setup and you can connect your edge devices and start creating dashboards! **Keep in mind**: the default development_values.yaml should only be used for development environments and never for production. See also notes below.

## Using it

You can now access Grafana and nodered via HTTP / HTTPS (depending on your setup). Default user for Grafana is admin. You can find the password in the secret RELEASE-NAME-grafana. Grafana is available via port 8080, nodered via 1880. 


