---
title: "1. Installation"
linkTitle: "1. Installation"
weight: 1
description: >
  This section explains how the system (edge and server) can be setup for development and testing enviroments.
---

There are three options to setup a development environment:
1. using a seperate device in combination with k3OS and our installation script (preferred)
2. using [minikube](https://kubernetes.io/en/docs/setup/minikube/) (recommended for developers working on the core functionalities of the stack)
3. manual installation (recommended for production enviroments, if you want to have fine grained control over the installation steps)

The focus of this article is to provide all necessary information to install it in a compressed tutorial. There are footnotes providing additional information on certain steps, that might be new to certain user groups.

## Option 1: using a seperate device in combination with k3OS and our installation script

Note: this content is also available in a presence workshop with an experienced facilitator guiding the participants through the installation and answering questions. Contact us for more information!

### Prerequisites

- a edge device with x86 architecture. We recommend using the [K300 from OnLogic](https://www.onlogic.com/eu-en/k300/)
- the [latest version of k3OS](https://github.com/rancher/k3os/releases/) installed on a bootable USB-stick [^flash-usb]. 
- a computer with SSH / SFTP client [^SSH-client] and [Lens](https://k8slens.dev/) (for accessing the Kubernetes cluster) installed. We recommend a laptop with an Ethernet port or with an Ethernet adapter. 
- local LAN (with DHCP) available via atleast two Ethernet cables and access to the internet.[^network-setup].
- a computer monitor connected with the edge device 
- a keyboard connected with the edge device

[^flash-usb]: LINK TO TECHNIQUES
[^SSH-client]: LINK TO TECHNIQUES
[^network-setup]: LINK TO TECHNIQUES WITH THIS IMAGE

{{< imgproc development-network.png Fit "500x300" >}}{{< /imgproc >}}


### Installation

This step is also available via a step-by-step video: **TODO**

#### k3OS

1. Insert your USB-stick with k3OS into your edge device and boot from it [^boot-usb]
2. [Install k3OS](TODO). When asked for a cloud-init file, enter this URL and confirm: `https://www.umh.app/development.yaml`. If you are paranoid or want to setup devices for production you could copy the file, modify and host it yourself.
3. If the installation fails with not beeing able to fetch the file above check the URL and the network configuration
4. If the installation fails with expired or untrusted certificates, [check out this guide](TODO).

Thats it! The device will automatically restart and download and install the stack. The installation has successfully started when you see the messages `factorycube-server deployed!` and `factorycube-edge deployed!`.

This process takes around 15 - 20 minutes depending on your internet connection and there will be no further information about the installation status on the output of the device visible (the information on the computer screen).

[^boot-usb]: LINK TO TECHNIQUES

#### Getting access to the device

To verify whether the installation worked and access [Grafana] (the dashboard) and [Node-RED], we will first enable SSH via password authentification, fetch the login details for [Kubernetes] and then login via [Lens].

##### Step 1: Login

The login console will look like "messed up" due to the logs of the installation process in the steps above. 

TODO: image

You can "clean it up" by pressing two times enter. 

TODO: image

You can also immediatly proceed with entering the default username `rancher` (do not forget to press enter) and the default password `rancher` to login. 

After a successfull login you should see the current IP address of the device on your computer screen.

TODO: image

#### Step 2: Enable SSH password authentification 

Enable SSH password authentification in k3OS [^ssh-password-authentication]. This is only necessary in development environments. For production environments we recommend using a certificate to authenticate, which is enabled by default. This can be archieved by modifying the cloud-init file and [linking to a public key stored on your GitHub account.](https://gist.github.com/dmangiarelli/1a0ae107aaa5c478c51e#ssh-setup-with-putty)

#### Step 3: Connect via SSH

Connect via SSH [^SSH-connect] from your laptop with the edge device. The IP address is shown on the computer screen on your edge device (see also step 1). If it is not available anymore, you can view the current IP address using `ip addr`. 

#### Step 4: Getting Kubernetes credentials

Execute `cat /etc/rancher/k3s/k3s.yaml` in your SSH session on your laptop  to retrieve the Kubernetes credentials

[Grafana]: https://grafana.com/
[Node-RED]: https://nodered.org/
[Kubernetes]: https://kubernetes.io/
[Lens]: https://k8slens.dev/

[^ssh-password-authentication]: /docs/tutorials/add-username-password-authentification-k3os-ssh/ 




# LEGACY

## Prerequisites

- k3os (AMD64) installed on a bootable USB-stick (you can get it here: https://github.com/rancher/k3os/releases/ ). You can create a bootable USB-stick using [balenaEtcher](https://www.balena.io/etcher/)
- a laptop with SSH / SFTP client (e.g. [MobaXTerm](https://mobaxterm.mobatek.net/)) and [Lens](https://k8slens.dev/) (for accessing the Kubernetes cluster) installed
- a edge device (currently only x86 systems supported)
- keyboard, monitor, cables
- A GitHub account with a public key. If you do not know how to do it [check out this tutorial]. You can download puttygen [here](https://the.earth.li/~sgtatham/putty/latest/w64/puttygen.exe)
- network setup and internet access according to the image below


## Steps

### k3OS

1. Install k3OS on your edge device using the bootable USB-stick (Press "entf or delete" repeatedly to enter the BIOS of the Factorycube and then boot from the USB stick with K3OS)
2. Choose the desired partition (in most cases 1)
3. Do not use a cloud configuration file
4. When asked, enter your GitHub username. In the future you will access the device via SSH with your private key. After the installation the system will reboot and show after successfull startup the IP adress of the device. If no IP is shown please check your network setup (especially whether you have DHCP activated). If you want to use the classic username / password authentification we recommend reading this article on [how to access SSH for username / password authentification in k3OS](../../Tutorials/add-username-password-authentification-k3os-ssh)
5. Configure K3OS as "server"
6. Remove the USB stick after the message that the system will restart in 5 seconds.
7. You can now disconnect Monitor and keyboard as you will do everything else via SSH.

### General setup

1. Connect via SSH e.g. with MobaXTerm (Username: rancher, Port: 22, remote host: ip of your edge device)
2. Authenticate with your private key which belongs to the to the public key stored in Github. (For MobaXTerm: Advanced SSH Settings -> private key and select your private key)
3. confirm the setting and connect
4. Install helm on your edge device
```bash
export VERIFY_CHECKSUM=false && curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3  && chmod 700 get_helm.sh && ./get_helm.sh
```
If this command fails with a `curl: (60) SSL certificate problem: certificate is not yet valid` (+ you are in a university or otherwise restricted network), [take a look here](../../tutorials/how-to-fix-ntp-issues/).

5. Clone or copy the content of the [united-manufacturing-hub repository on Github](https://github.com/united-manufacturing-hub/united-manufacturing-hub) into the home folder (`/home/rancher/united-manufacturing-hub`). You can use the following command to do that for you (you might need to adjust the version number): `curl -L https://github.com/united-manufacturing-hub/united-manufacturing-hub/tarball/v0.4.2 | tar zx && mv $(find . -maxdepth 1  -type d -name "united-manufacturing-hub*") united-manufacturing-hub`
6. Execute `cat /etc/rancher/k3s/k3s.yaml` to retrieve the secrets to connect to your Kubernetes cluster
7. Paste the file into Lens when adding a new cluster and adjust the IP 127.0.0.1 (only change the IP address. The port, the numbers after the colon, remain the same). You should now see the cluster in Lens.
8. Create two namespaces in your Kubernetes cluster called `factorycube-edge` and `factorycube-server` by executing the following command:
```bash
kubectl create namespace factorycube-edge && kubectl create namespace factorycube-server
```

### Install factorycube-edge

Warning: in production you should use your own certificates and not the ones provided by us as this is highly insecure when exposed to the internet and any insecure network. A tutorial to setup PKI infrastructure for MQTT according to [this guide](../../tutorials/pki)

1. Go into the folder `/home/rancher/united-manufacturing-hub/deployment/factorycube-edge`
2. Create a new file called `development_values.yaml` using `touch development_values.yaml`
3. Copy the following content to that file or use the following example [`development_values.yaml`](/examples/factorycube-server/development_values.yaml). 
4. **(Only if you did not use example file)** Copy the certificates in `deplotment/factorycube-server/developmentCertificates/pki/` and then `ca.crt`, `issued/TESTING.crt` and `issued/private/TESTING.key` into `development_values.yaml`. Additionally use as `mqttBridgeURL` `ssl://factorycube-server-vernemq-local-service.factorycube-server:8883`. 
5. Adjust `iprange` to your network IP range

Example for `development_values.yaml`:
```yaml
mqttBridgeURL: "ssl://mqtt.umh.app:8883"
mqttBridgeTopic: "ia/factoryinsight"
sensorconnect:
  iprange: "172.16.1.0/24"
mqttBridgeCACert: |
  ENTER CERT HERE
mqttBridgeCert: |
  ENTER CERT HERE
mqttBridgePrivkey: |
  ENTER CERT HERE
```

6. Execute `helm install factorycube-edge /home/rancher/united-manufacturing-hub/deployment/factorycube-edge --values "/home/rancher/united-manufacturing-hub/deployment/factorycube-edge/development_values.yaml" --set serialNumber=$(hostname) --kubeconfig /etc/rancher/k3s/k3s.yaml  -n factorycube-edge` (change kubeconfig and serialNumber accordingly) (Please pay attention to the correct path. The path may be /home/rancher/united-manufacturing-hub-main ... or similar)

### Install factorycube-server

Warning: in production this should be installed on a seperate device / in the cloud to ensure High Availability and provide automated backups. 

1. Configure values.yaml according to your needs. For the development version you do not need to do anything. For help in configuring you can take a look into the respective documentation of the subcharts ([Grafana](https://github.com/grafana/helm-charts), [redis](https://github.com/bitnami/charts/tree/master/bitnami/redis), [timescaleDB](https://github.com/timescale/timescaledb-kubernetes/tree/master/charts/timescaledb-single), [verneMQ](https://github.com/vernemq/docker-vernemq/tree/master/helm/vernemq)) or into the documentation of the subcomponents ([factoryinsight](../../developers/factorycube-server/factoryinsight), [mqtt-to-postgresql](../../developers/factorycube-server/mqtt-to-postgresql))
2. Execute `helm install factorycube-server /home/rancher/united-manufacturing-hub/deployment/factorycube-server --values "/home/rancher/united-manufacturing-hub/deployment/factorycube-server/values.yaml" --kubeconfig /etc/rancher/k3s/k3s.yaml -n factorycube-server` and wait. Helm will automatically install the entire stack across multiple node. It can take up to several minutes until everything is setup. (Please pay attention to the correct path. The path may be /home/rancher/united-manufacturing-hub-main ... or similar)

Everything should be now successfully setup and you can connect your edge devices and start creating dashboards! **Keep in mind**: the default development_values.yaml should only be used for development environments and never for production. See also notes below.

## Using it

You can now access Grafana and nodered via HTTP / HTTPS (depending on your setup). Default user for Grafana is admin. You can find the password in the secret RELEASE-NAME-grafana. Grafana is available via port 8080, nodered via 1880. 

