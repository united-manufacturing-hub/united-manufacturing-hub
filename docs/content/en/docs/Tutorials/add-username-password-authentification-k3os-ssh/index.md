---
title: "How to enable SSH password authentification in k3OS"
linkTitle: "How to enable SSH password authentification in k3OS"
description: >
  This article explains how to enable the classic username / password authentification for SSH in k3os 
---

## Prerequisites

- Edge device running [k3OS](https://github.com/rancher/k3os)
- SSH access to that device

## Tutorial

1. Access the edge device via SSH
2. Set the value `PasswordAuthentication` in the file `/etc/ssh/sshd_config` to `yes`. You can use the command `sudo nano /etc/ssh/sshd_config` to do that. If you are unsure how to use nano you can take a look into [this guide](https://staffwww.fullcoll.edu/sedwards/Nano/IntroToNano.html)
4. Reboot the device

