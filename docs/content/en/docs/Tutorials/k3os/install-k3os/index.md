---
title: "How to install k3OS"
linkTitle: "How to install k3OS"
aliases:
    - /docs/tutorials/install-k3os/
description: >
  This article explains how to install k3OS on an edge device using the United Manufacturing Hub installation script.
---

## Prerequisites

- edge device with keyboard and computer screen successfully booted from a USB-stick (see also [Installation](/docs/getting-started/setup-development/))
- you should see on the computer screen the screen below (it will automatically continue after 10 seconds with the installation, so do not worry if you only see it for some seconds)

{{< imgproc 1.png Fit "800x500" >}}boot menu of k3OS{{< /imgproc >}}

## Tutorial

Wait until k3OS is fully started. You should see the screen below:
{{< imgproc 2.png Fit "800x500" >}}k3OS installer fully booted{{< /imgproc >}}

Enter `rancher` and press enter to login.

{{< imgproc 3.png Fit "800x500" >}}k3OS installer fully booted with rancher as username{{< /imgproc >}}

You should now be logged in. 

Pro tip: Execute `lsblk` and identify your hard drive (e.g., by the size). It will prevent playing russian roulette on a later step.

Now type in `sudo k3os install` to start the installation process.

{{< imgproc 4.png Fit "800x500" >}}entered `sudo k3os install`{{< /imgproc >}}

You are now prompted to select what you want to install. Select `1` and press enter or just press enter (the stuff in brackets [] is the default configuration if you do not specify anything and just press enter).

{{< imgproc 5.png Fit "800x500" >}}Install to disk{{< /imgproc >}}

At this step the system might ask you to select your hard drive. One of the devices `sda` or `sdb` will be your hard drive and one the USB-stick you booted from. If you do not know what your hard drive is, you need to play russian roulette and select one device. If you find out later, that you accidently installed it onto the USB-stick, then repeat the installation process and use the other device.

After that select `y` when you get asked for a cloud-init file

{{< imgproc 6.png Fit "800x500" >}}Configure system with cloud-init file{{< /imgproc >}}

Now enter the URL of your cloud-init file, e.g., the one mentioned in the [Installation guide](/docs/getting-started/setup-development/). 

Press enter to continue.

{{< imgproc 7.png Fit "800x500" >}}Specify the cloud-init file{{< /imgproc >}}
{{< imgproc 8.png Fit "800x500" >}}example (do not use this URL){{< /imgproc >}}

Confirm with `y` and press enter.

{{< imgproc 9.png Fit "800x500" >}}Confirm installation with `y`{{< /imgproc >}}

{{< imgproc 11.png Fit "800x500" >}}Confirm installation with `y`{{< /imgproc >}}

If the installation fails with not beeing able to fetch the cloud-init file check the URL and the network configuration

If the installation fails with expired or untrusted certificates (`curl: (60) SSL certificate problem: certificate is not yet valid` or similar), [check out this guide](/docs/tutorials/k3os/how-to-fix-invalid-certs-due-to-misconfigured-date/).

The device will then reboot. You might want to remove the USB-stick to prevent booting from the USB-stick again.

If the following screen appears you did everything correct and k3OS was successfully installed.

{{< imgproc 10.png Fit "800x500" >}}{{< /imgproc >}}

