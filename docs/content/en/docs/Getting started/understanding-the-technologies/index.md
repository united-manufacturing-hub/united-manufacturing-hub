---
title: "0. Understanding the technologies"
linkTitle: "0. Understanding the technologies"
weight: 1
description: >
  Strongly recommended. This section gives you an introduction into the used technologies. A rough understanding of these technologies is fundamental for installing and working with the system. Additionally, this article provides further learning materials for certain technologies.
---

The materials presented below are usually teached in a 2-3 h workshop session on a live production shopfloor at the Digital Capability Center Aachen. You can find the outline further below. 

## Introduction into IT / OT

### History: IT & OT were typically seperate silos but are currently converging to IIoT

### Operational Technology (OT)

#### OT connects own set of various technologies to create highly reliable and stable machines

#### The concepts of OT are close to eletronics and with a strong focus on human and machine safety

#### OT focuses on handling processes with highest possible safety for machines and operators

#### Typical device architecture and situation for the OT

#### Fundamentals 1: Programmable Logic Controller (PLC)

#### Fundamentals 2: PLCs & PLC programming

#### Fundamentals 3: Process control using PLCs

### Information Technology (IT)

#### IT connects millions of devices and manages their data flows

#### The concepts of IT are focusing on digital data and networks

#### What is important in IT? What is not important?

#### Fundamentals 1: Networking

#### Fundamentals 2: Cloud and Microservices

#### Fundamentals 3: How microservices are built: Docker in 100 seconds

#### Fundamentals 4: How to orchestrate IT stacks: Kubernetes in 100 seconds

#### Fundamentals 5: Typical network setups in production facilities

### Industrial Internet of Things (IIoT)

#### Whats it's all about

#### A full digital transformation of manufacturing needs to consider business, technology and organization

#### IIoT sits at the intersection of IT and OT

#### Architecting for scale

#### Best-practices 1: Avoid common traps in the IIoT space

#### Best-practices 2: Protocols which allow communication of IT and OT systems

#### Best-practices 3: Reduce complexity in machine connection with tools like Node-RED

#### Best-practices 4: Connect IT and OT securely using a Demilitarized Zone (DMZ)

Link to blog

#### Architecture

See concepts

#### Example projects

Link to projects

## Deep-dive: IT technologies and techniques

The section focuses on various deep-dives into IT technologies and techniques. This is not required to read fully and only accessed when you need help with certain things.

### Flashing a operating system onto a USB-stick

There are multiple ways to flash a operating system onto a USB-stick. We will present you the method of using [balenaEtcher](https://www.balena.io/etcher/).

#### Prerequisites

- You need a USB-stick (we recommend USB 3.0 for better speed)
- You need a OS image in the *.iso format. For k3OS you could choose for example [this version](https://github.com/rancher/k3os/releases/download/v0.20.7-k3s1r0/k3os-amd64.iso)

#### Steps

{{< imgproc balena_1.png Fit "800x500" >}}Download balenaEtcher: www.balena.io/etcher/ {{< /imgproc >}}
{{< imgproc balena_2.png Fit "800x500" >}}Insert USB-stick and open balenaEtcher{{< /imgproc >}}
{{< imgproc balena_3.png Fit "800x500" >}}Select downloaded *.iso by clicking on "Flash from file" (the sceeen might look different based on your operating system){{< /imgproc >}}
{{< imgproc balena_4.png Fit "800x500" >}}Select the USB-stick by clicking on "Select target"{{< /imgproc >}}
{{< imgproc balena_5.png Fit "800x500" >}}Select "Flash"{{< /imgproc >}}
{{< imgproc balena_6.png Fit "800x500" >}}It will flash the image on the USB-stick{{< /imgproc >}}
{{< imgproc balena_7.png Fit "800x500" >}}You are done!{{< /imgproc >}}

These steps are also available as a YouTube tutorial from the user kilObit.
{{< youtube kWRx40Q8B_A >}}

### Installing operating systems from a USB-stick

#### Prerequisites

- you need a bootable USB-stick (see also [flashing-a-operating-system-onto-a-usb-stick](#flashing-a-operating-system-onto-a-usb-stick))

#### Steps

1. Plug the USB-stick into the device
2. Reboot
3. Press the button to go into the boot menu. This step is different for every hardware and is described in the hardware manual. If you do not want to look it up you could try smashing the following buttons during booting (the stuff before the operating system is loaded) and hope for the best: F1, F2, F11, F12, delete
4. Once you are in the boot menu, select to boot from the USB-stick

### Connecting with SSH

#### For Windows

We recommend MobaXTerm. **TODO**

#### For Linux

For Linux you can typically use the inbuilt commands to connect with a device via SSH. Connect using the following command:

`ssh <username>@<IP>`, e.g., `ssh rancher@192.168.99.118`.

{{< imgproc SSH_linux_1.png Fit "800x500" >}}Connect via SSH{{< /imgproc >}}

There will be a warning saying that the authenticity of the host can't be established. Enter `yes` to continue with the connection.

{{< imgproc SSH_linux_2.png Fit "800x500" >}}Warning message: The authenticity of host 'xxx' can't be established.{{< /imgproc >}}

Enter the password and press enter. The default password of the auto setup will be `rancher`.

{{< imgproc SSH_linux_3.png Fit "800x500" >}}Successfully logged in via SSH{{< /imgproc >}}

### Development network

{{< imgproc development-network.png Fit "500x300" >}}{{< /imgproc >}}

### Versioning

In IT Semantic Versioning has established itself as the standard to describe versions. It consists out of the format `MAJOR.MINOR.PATCH`, e.g., `1.0.0`. 

`MAJOR` is incremented when making incompatible API changes.

`MINOR` is incremented when you add functionality

`PATCH` is incremented when you make bug fixes

If the version is followed by a '-' sign, then it means it is a pre-release and not stable yet. **Therefore, the latest stable version means the highest version available that is not a pre-release / has no '-' sign.**

More information can be found in the [specification of Semantic Versioning 2.0](https://semver.org/).

### ...

## Deep Dive: OT technologies and techniques

### ...

