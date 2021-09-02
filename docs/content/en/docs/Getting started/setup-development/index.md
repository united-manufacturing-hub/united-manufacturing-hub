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
3. manual installation (recommended for production environments, if you want to have fine grained control over the installation steps)

**The focus of this article is to provide all necessary information to install it in a compressed tutorial. There are footnotes providing additional information on certain steps, that might be new to certain user groups.**

## Option 1: using a seperate device in combination with k3OS and our installation script

Note: this content is also available in a presence workshop with an experienced facilitator guiding the participants through the installation and answering questions. Contact us for more information!

### Prerequisites

- a edge device with x86 architecture. We recommend using the [K300 from OnLogic](https://www.onlogic.com/eu-en/k300/)
- the [latest version of k3OS](https://github.com/rancher/k3os/releases/) [^versioning] installed on a bootable USB-stick [^flash-usb]. 
- a computer with SSH / SFTP client [^SSH-client] and [Lens](https://k8slens.dev/) (for accessing the Kubernetes cluster) installed. We recommend a laptop with an Ethernet port or with an Ethernet adapter. 
- local LAN (with DHCP) available via atleast two Ethernet cables and access to the internet.[^network-setup].
- a computer monitor connected with the edge device 
- a keyboard connected with the edge device

[^flash-usb]: See also out guide: [How to flash an operating system on a USB-stick](/docs/getting-started/understanding-the-technologies/#flashing-a-operating-system-onto-a-usb-stick)
[^SSH-client]: See also out guide: [How to connect via SSH](/docs/getting-started/understanding-the-technologies/#connecting-with-ssh)
[^network-setup]: See also out guide: [How to setup a development network](/docs/getting-started/understanding-the-technologies/#development-network)
[^versioning]: See also out guide: [What is semantic versioning](/docs/getting-started/understanding-the-technologies/#versioning)



### Installation

This step is also available via a step-by-step video: **TODO**

#### k3OS

1. Insert your USB-stick with k3OS into your edge device and boot from it [^boot-usb]
2. [Install k3OS](/docs/tutorials/install-k3os/). When asked for a cloud-init file, enter this URL and confirm: `https://www.umh.app/development.yaml`. If you are paranoid or want to setup devices for production you could copy the file, modify and host it yourself. [Here is the template](/examples/development.yaml)

This process takes around 15 - 20 minutes depending on your internet connection and there will be no further information about the installation status on the output of the device visible (the information on the computer screen).

[^boot-usb]: See also out guide: [How to boot from a USB-stick](/docs/getting-started/understanding-the-technologies/#installing-operating-systems-from-a-usb-stick)

#### Getting access to the device

To verify whether the installation worked and access [Grafana] (the dashboard) and [Node-RED], we will first enable SSH via password authentification, fetch the login details for [Kubernetes] and then login via [Lens].

[Grafana]: https://grafana.com/
[Node-RED]: https://nodered.org/
[Kubernetes]: https://kubernetes.io/
[Lens]: https://k8slens.dev/

##### Step 1: Login

The login console will look like "messed up" due to the logs of the installation process in the steps above. 

{{< imgproc 1.png Fit "800x500" >}}Immediatly after start. Nothing is messed up yet.{{< /imgproc >}}
{{< imgproc 2.png Fit "800x500" >}}"Messed up" login screen{{< /imgproc >}}

You can "clean it up" by pressing two times enter. 

You can also immediatly proceed with entering the default username `rancher` (do not forget to press enter) and the default password `rancher` to login. 
{{< imgproc 3.png Fit "800x500" >}}Logging into k3OS using the username `rancher`{{< /imgproc >}}
{{< imgproc 4.png Fit "800x500" >}}Logging into k3OS using the password `rancher`{{< /imgproc >}}

After a successfull login you should see the current IP address of the device on your computer screen.

{{< imgproc 5.png Fit "800x500" >}}Successfully logged into k3OS{{< /imgproc >}}


#### Step 2: Enable SSH password authentification 

Enable SSH password authentification in k3OS [^ssh-password-authentication]. This is currently not necessary anymore as the automated setup script will do that automatically, **therefore this step can be skipped**. This paragraph only exists to remind you that this setting is not the default behavior of k3OS and should be deactivated in production environments.

For production environments we recommend using a certificate to authenticate, which is enabled by default. This can be archieved by modifying the cloud-init file and [linking to a public key stored on your GitHub account.](https://gist.github.com/dmangiarelli/1a0ae107aaa5c478c51e#ssh-setup-with-putty)

[^ssh-password-authentication]: See also out guide: [Enabling k3os password authentication](/docs/tutorials/add-username-password-authentification-k3os-ssh/)

#### Step 3: Connect via SSH

Connect via SSH [^SSH-client] from your laptop with the edge device. The IP address is shown on the computer screen on your edge device (see also step 1). If it is not available anymore, you can view the current IP address using `ip addr`. 

#### Step 4: Getting Kubernetes credentials

Execute `cat /etc/rancher/k3s/k3s.yaml` in your SSH session on your laptop  to retrieve the Kubernetes credentials. Copy the content of the result into your clipboard.

{{< imgproc k3s_secret_1.png Fit "800x500" >}}Execute `cat /etc/rancher/k3s/k3s.yaml`{{< /imgproc >}}

{{< imgproc k3s_secret_2.png Fit "800x500" >}}Copy the content{{< /imgproc >}}

Connect with the edge device and the Kubernetes credentials in your clipboard.

{{< imgproc k3s_secret_3.png Fit "800x500" >}}Add a new cluster in Lens{{< /imgproc >}}
{{< imgproc k3s_secret_4.png Fit "800x500" >}}Select `Paste as text`{{< /imgproc >}}
{{< imgproc k3s_secret_5.png Fit "800x500" >}}Paste from the clipboard{{< /imgproc >}}
{{< imgproc k3s_secret_6.png Fit "800x500" >}}{{< /imgproc >}}

Ensure that you have adjusted the IP in the Kubernetes credentials with the IP of the edge device.

{{< imgproc k3s_secret_7.png Fit "800x500" >}}{{< /imgproc >}}
{{< imgproc k3s_secret_8.png Fit "800x500" >}}{{< /imgproc >}}

You have now access to the Kubernetes cluster!

### Verifying the installation and extracting credentials to connect with the dashboard

The installation is finished when all Pods are "Running". You can do that by clicking on Pods on the left side.

Some credentials are automatically generated by the system. One of them are the login credentials of Grafana. You can retrieve them by clicking on "Secrets" on the left side in [Lens]. Then search for a secret called "grafana-secret" and open it. Press "decode" and copy the password into your clipboard.

### Opening Grafana and Node-RED

Grafana is now accessible by opening the following URL in your browser: `http://<IP>:8080` (e.g., `http://192.168.1.2:8080`). You can login by using the username `admin` and password from your clipboard.

Node-RED is accessible by opening the following URL in your browser: `http://<IP>:1880/nodered`.

Once you have access, you can proceed with the second article [Connecting machines and creating dashboards](/docs/getting-started/connecting-machines-creating-dashboards/).

## Option 2: using minikube

**TODO**


## Option 3: manual installation

For a manual installation, we recommend that you take a look at the installation script and follow these commands manually and adjust them when needed. 
