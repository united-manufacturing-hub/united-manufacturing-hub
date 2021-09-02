---
title: "How to enable SSH password authentification in k3OS"
linkTitle: "How to enable SSH password authentification in k3OS"
description: >
  This article explains how to enable the classic username / password authentification for SSH in k3os 
---

**DANGER: NOT RECOMMENDED FOR PRODUCTION! USE DEFAULT BEHAVIOR WITH CERTIFICATES INSTEAD**

## Prerequisites

- Edge device running [k3OS](https://github.com/rancher/k3os)
- SSH access to that device

## Tutorial

1. Access the edge device via SSH
2. Set the value `PasswordAuthentication` in the file `/etc/ssh/sshd_config` to `yes` and restart the service `sshd`. You can use the following command:

```bash
sudo vim /etc/ssh/sshd_config -c "%s/PasswordAuthentication  no/PasswordAuthentication  yes/g | write | quit" && sudo service sshd restart

```
