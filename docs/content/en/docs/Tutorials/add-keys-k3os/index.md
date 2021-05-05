---
title: "How to add additional SSH keys in k3OS"
linkTitle: "How to add additional SSH keys in k3OS"
description: >
  This article explains how to add an additional SSH key to k3OS, so that multiple people can access the device
---

## Prerequisites

- Edge device running [k3OS](https://github.com/rancher/k3os)
- SSH access to that device
- SSH / SFTP client
- Public and private key suited for SSH access

## Tutorial

1. Access the edge device via SSH
2. Go to the folder `/home/rancher/.ssh` and edit the file `authorized_keys`
3. Add there your additional SSH key
