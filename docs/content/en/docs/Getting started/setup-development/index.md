---
title: "1. Installation"
linkTitle: "1. Installation"
weight: 1
description: >
  This section explains how the system (edge and server) can be setup for development and testing enviroments.
---

There are three options to setup a development environment:
1. using a seperate device in combination with k3OS and our installation script (preferred). This requires an external device and is a fully automated installation.
2. using [minikube](https://kubernetes.io/de/docs/setup/minikube/) (recommended for developers working on the core functionalities of the stack). This method allows you to install the stack on your device and is semi-automated.
3. manual installation (recommended for production environments, if you want to have fine grained control over the installation steps). This can be executed either on an external device or on your device.

**The focus of this article is to provide all necessary information to install it in a compressed tutorial. There are footnotes providing additional information on certain steps, that might be new to certain user groups.**

## Option 1: using a seperate device in combination with k3OS and our installation script

Note: this content is also available in a presence workshop with an experienced facilitator guiding the participants through the installation and answering questions. Contact us for more information!
 
### Prerequisites

{{< imgproc prerequisites_k3os.png Fit "1280x500" >}}{{< /imgproc >}}

This installation method requires some previous settings:

- an edge device with x86 architecture. We recommend using the [K300 from OnLogic](https://www.onlogic.com/eu-en/k300/)
- the [latest version of k3OS](https://github.com/rancher/k3os/releases/) [^versioning] installed on a bootable USB-stick [^flash-usb]. 
- a computer with SSH / SFTP client [^SSH-client] and [Lens](https://k8slens.dev/) (for accessing the Kubernetes cluster) installed. We recommend a laptop with an Ethernet port or with an Ethernet adapter. 
- local LAN (with DHCP) available via atleast two Ethernet cables and access to the internet.[^network-setup].
- a computer monitor connected with the edge device 
- a keyboard connected with the edge device

**As shown, the Factorycube is an optional device that combines all the required hardware in a rugged industrial gateway for industrial use.**

[^flash-usb]: See also out guide: [How to flash an operating system on a USB-stick](/docs/tutorials/general/flashing-operating-system-on-usb)
[^SSH-client]: See also out guide: [How to connect via SSH](/docs/tutorials/general/connect-with-ssh)
[^network-setup]: See also out guide: [How to setup a development network](/docs/tutorials/general/networking)
[^versioning]: See also out guide: [What is semantic versioning](/docs/tutorials/general/versioning)

### Installation

This step is also available via a step-by-step video: **TODO**

#### k3OS

1. Insert your USB-stick with k3OS into your edge device and boot from it [^boot-usb]
2. [Install k3OS](/docs/tutorials/k3os/install-k3os/). When asked for a cloud-init file, enter this URL and confirm: `https://www.umh.app/development.yaml`. If you are paranoid or want to setup devices for production you could copy the file, modify and host it yourself. [Here is the template](/examples/development.yaml)

This process takes around 15 - 20 minutes depending on your internet connection and there will be no further information about the installation status on the output of the device visible (the information on the computer screen).

[^boot-usb]: See also out guide: [How to boot from a USB-stick](/docs/tutorials/general/install-operating-system-from-usb)

#### Getting access to the device

To verify whether the installation worked and to access [Grafana] (the dashboard) and [Node-RED], we will first enable SSH via password authentification, fetch the login details for [Kubernetes] and then login via [Lens].

[Grafana]: https://grafana.com/
[Node-RED]: https://nodered.org/
[Kubernetes]: https://kubernetes.io/
[Lens]: https://k8slens.dev/

##### Step 1: Login

The login console will look like "messed up" due to the logs of the installation process in the steps above. 

{{< imgproc 1.png Fit "1280x500" >}}Immediatly after start. Nothing is messed up yet.{{< /imgproc >}}
{{< imgproc 2.png Fit "1280x500" >}}"Messed up" login screen{{< /imgproc >}}

You can "clean it up" by pressing two times enter. 

You can also immediatly proceed with entering the default username `rancher` (do not forget to press enter) and the default password `rancher` to login. 
{{< imgproc 3.png Fit "1280x500" >}}Logging into k3OS using the username `rancher`{{< /imgproc >}}
{{< imgproc 4.png Fit "1280x500" >}}Logging into k3OS using the password `rancher`{{< /imgproc >}}

After a successfull login you should see the current IP address of the device on your computer screen.

{{< imgproc 5.png Fit "1280x500" >}}Successfully logged into k3OS{{< /imgproc >}}


#### Step 2: Enable SSH password authentification 

Enable SSH password authentification in k3OS [^ssh-password-authentication]. This is currently not necessary anymore as the automated setup script will do that automatically, **therefore this step can be skipped**. This paragraph only exists to remind you that this setting is not the default behavior of k3OS and should be deactivated in production environments.

For production environments we recommend using a certificate to authenticate, which is enabled by default. This can be archieved by modifying the cloud-init file and [linking to a public key stored on your GitHub account.](https://gist.github.com/dmangiarelli/1a0ae107aaa5c478c51e#ssh-setup-with-putty)

[^ssh-password-authentication]: See also out guide: [Enabling k3os password authentication](/docs/tutorials/k3os/add-username-password-authentification-k3os-ssh/)

#### Step 3: Connect via SSH

Connect via SSH [^SSH-client] from your laptop with the edge device. The IP address is shown on the computer screen on your edge device (see also step 1). If it is not available anymore, you can view the current IP address using `ip addr` or `ifconfig eth0` (works with out devices). 

Username: `rancher`
Password: `rancher`

#### Step 4: Getting Kubernetes credentials

{{< alert title="Note" color="info">}}
For experts: in the folder `/tools` you can find a bash script that will do this step automatically for you 
{{< /alert >}}

Execute `cat /etc/rancher/k3s/k3s.yaml` in your SSH session on your laptop  to retrieve the Kubernetes credentials. Copy the content of the result into your clipboard.

{{< imgproc k3s_secret_1.png Fit "1280x500" >}}Execute `cat /etc/rancher/k3s/k3s.yaml`{{< /imgproc >}}

{{< imgproc k3s_secret_2.png Fit "1280x500" >}}Copy the content{{< /imgproc >}}

Connect with the edge device using the software [Lens] and the Kubernetes credentials from your clipboard.

{{< imgproc k3s_secret_3.png Fit "1280x500" >}}Add a new cluster in Lens{{< /imgproc >}}
{{< imgproc k3s_secret_4.png Fit "1280x500" >}}Select `Paste as text`{{< /imgproc >}}
{{< imgproc k3s_secret_5.png Fit "1280x500" >}}Paste from the clipboard{{< /imgproc >}}
{{< imgproc k3s_secret_6.png Fit "1280x500" >}}{{< /imgproc >}}

Ensure that you have adjusted the IP in the Kubernetes credentials with the IP of the edge device.

Also make sure that you simply adjust the IP in between. The port that follows the `:` should remain untouched (e.g. https://XXX.X.X.X:**6443** in that case).

**Hint:** If you get the message 'certificate not valid' or something similar when connecting, verify that you entered the correct port before proceeding to the troubleshooting section (/docs/tutorials/k3os/how-to-fix-invalid-certs-due-to-misconfigured-date/).

{{< imgproc k3s_secret_7.png Fit "1280x500" >}}{{< /imgproc >}}
{{< imgproc k3s_secret_8.png Fit "1280x500" >}}{{< /imgproc >}}

You have now access to the Kubernetes cluster!

### Verifying the installation and extracting credentials to connect with the dashboard

The installation is finished when all Pods are "Running". You can do that by clicking on Pods on the left side.

{{< imgproc verify_1.png Fit "1280x500" >}}Click in Lens on Workloads and then on Pods{{< /imgproc >}}

{{< imgproc verify_2.png Fit "1280x500" >}}Select the relevant namespaces `factorycube-server` and `factorycube-edge`{{< /imgproc >}}

{{< imgproc verify_3.png Fit "1280x500" >}}everything should be running{{< /imgproc >}}

Some credentials are automatically generated by the system. One of them are the login credentials of Grafana. You can retrieve them by clicking on "Secrets" on the left side in [Lens]. Then search for a secret called "grafana-secret" and open it. Press "decode" and copy the password into your clipboard.

{{< imgproc verify_4.png Fit "1280x500" >}}Press on the left side in Lens on Configuration and then Secret.{{< /imgproc >}}

{{< imgproc verify_5.png Fit "1280x500" >}}Then select grafana-secret{{< /imgproc >}}

{{< imgproc verify_6.png Fit "1280x500" >}}Then click on the eye on the right side of adminpassword to decrypt it{{< /imgproc >}}

### Opening Grafana and Node-RED

Grafana is now accessible by opening the following URL in your browser: `http://<IP>:8080` (e.g., `http://192.168.1.2:8080`). You can login by using the username `admin` and password from your clipboard.

Node-RED is accessible by opening the following URL in your browser: `http://<IP>:1880/nodered`.

**Once you have access, you can proceed with the second article [Connecting machines and creating dashboards](/docs/getting-started/connecting-machines-creating-dashboards/).**

## Option 2: using minikube

This option is only recommended for developers. Therefore, the installation is targeted for them and might not be as detailed as option 1.

### Prerequisites

- minikube installed according to the [official documentation](https://minikube.sigs.k8s.io/docs/start/)
- repository cloned using `git clone https://github.com/united-manufacturing-hub/united-manufacturing-hub.git` or downloaded and extracted using the download button on GitHub.
- helm [^helm] and kubectl [^kubectl] installed

[^helm]: Can be installed using the following command: `export VERIFY_CHECKSUM=false && curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3  && chmod 700 get_helm.sh && ./get_helm.sh`
[^kubectl]: Can be installed on Ubuntu using the following command: `sudo apt-get install kubectl`

### Steps

1. Start minikube using `minikube start`. If minikube fails to start, see the [drivers page](https://minikube.sigs.k8s.io/docs/drivers/) for help setting up a compatible container or virtual-machine manager.
{{< imgproc minikube_1.png Fit "1280x500" >}}Output of the command `minikube start`{{< /imgproc >}}
2. If everything went well, kubectl is now configured to use the minikube cluster by default. `kubectl version` should look like in the screenshot.
{{< imgproc minikube_2.png Fit "1280x500" >}}Expected output of `kubectl version`{{< /imgproc >}}
3. Go into the cloned repository and into the folder `deployment/united-manufacturing-hub`
4. execute: `kubectl create namespace united-manufacturing-hub`
{{< imgproc minikube_3.png Fit "1280x500" >}}Expected output of `kubectl create namespace`{{< /imgproc >}}
{{< imgproc minikube_4.png Fit "1280x500" >}}Output of curl{{< /imgproc >}}
6. Install by executing the following command: `helm install united-manufacturing-hub . -n united-manufacturing-hub`
{{< imgproc minikube_5.png Fit "1280x500" >}}Output of `helm install`{{< /imgproc >}}

Now go grab a coffee and wait 15-20 minutes until all pods are "Running".

Now you should be able to see the cluster using [Lens]

## Option 3: manual installation

For a manual installation, we recommend that you take a look at the installation script and follow these commands manually and adjust them when needed. 
