---
title: "Setting up the PKI infrastructure"
linkTitle: "Setting up the PKI infrastructure"
weight: 2
description: >
  This document describes how to create and manage the certificates required for MQTT 
---

## Prerequisites

- Installed easy-rsa (https://github.com/OpenVPN/easy-rsa)

This tutorial is assuming zou are using ubuntu and have installed easy-rsa using `sudo apt-get install easyrsa`

## Initially setting up the infrastructure

Create a new directory and go into it, e.g. 
```bash
mkdir ~/mqtt.umh.app/
cd ~/mqtt.umh.app/
```

Setup basic PKI infrastructure with `/usr/share/easy-rsa/easyrsa init-pki`

Copy the default configuration file with `cp /usr/share/easy-rsa/vars.example pki/vars` and edit it to your liking (e.g. adjust EASYRSA_REQ_... and CA and cert validity)

Build the CA using `/usr/share/easy-rsa/easyrsa build-ca nopass`

Create the server certificate by using the following commands (exchange mqtt.umh.app with your domain!):
```bash
/usr/share/easy-rsa/easyrsa gen-req mqtt.umh.app nopass
/usr/share/easy-rsa/easyrsa sign-req server mqtt.umh.app 

Copy the private key `pki/private/mqtt.umh.app.key` and the public certificate `pki/issued/mqtt.umh.app.crt` together with the root CA `pki/ca.crt` to the configuration of the MQTT broker.

## Adding new clients

Create new clients with following commands (remember to change TESTING with the planned MQTT client id):
```bash
./easyrsa gen-req TESTING nopass
./easyrsa sign-req client TESTING
```
