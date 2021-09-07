---
title: "How to enable SSH password authentification in k3OS"
linkTitle: "How to enable SSH password authentification in k3OS"
description: >
  This article explains how to enable the classic username / password authentification for SSH in k3os 
---

**DANGER: NOT RECOMMENDED FOR PRODUCTION! USE DEFAULT BEHAVIOR WITH CERTIFICATES INSTEAD**

By default, k3OS allows SSH connections only using certificates. This is a much safer method than using passwords. However, we realized that most mechanical engineers and programmers are overwhelmed with the creation of a [public key infrastructure](/docs/tutorials/pki/). Therefore, it might make sense to enable password authentication in k3OS for development mode.

## Prerequisites

- Edge device running [k3OS](https://github.com/rancher/k3os)
- physical access to that device

## Tutorial

1. Access the edge device via computer screen and keyboard and login with username `rancher` and `rancher`
2. Set the value `PasswordAuthentication` in the file `/etc/ssh/sshd_config` to `yes` and restart the service `sshd`. You can use the following command:

```bash
sudo vim /etc/ssh/sshd_config -c "%s/PasswordAuthentication  no/PasswordAuthentication  yes/g | write | quit" && sudo service sshd restart
```
