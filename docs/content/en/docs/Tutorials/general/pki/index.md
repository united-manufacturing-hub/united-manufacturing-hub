---
title: "Setting up the PKI infrastructure"
linkTitle: "Setting up the PKI infrastructure"
aliases:
    - /docs/tutorials/pki/
weight: 2
description: >
  This document describes how to create and manage the certificates required for MQTT 
---

## Prerequisites

- Installed easy-rsa (https://github.com/OpenVPN/easy-rsa)
- basic knowledge about public key infrastructure. [We recommend reading our article first](/docs/Concepts/symmetric-asymmetric-encryption)

This tutorial is assuming zou are using ubuntu and have installed easy-rsa using `sudo apt-get install easyrsa`

## Initially setting up the infrastructure

Create a new directory and go into it, e.g. 
```bash
mkdir ~/mqtt.umh.app/
cd ~/mqtt.umh.app/
```

Enable batch mode of easyrsa with `export EASYRSA_BATCH=1`

Setup basic PKI infrastructure with `/usr/share/easy-rsa/easyrsa init-pki`

Copy the default configuration file with `cp /usr/share/easy-rsa/vars.example pki/vars` and edit it to your liking (e.g. adjust EASYRSA_REQ_... and CA and cert validity)

Build the CA using `export EASYRSA_REQ_CN=YOUR_CA_NAME && /usr/share/easy-rsa/easyrsa build-ca nopass`. Replace `YOUR_CA_NAME` with a name for your certificate authority (CA), e.g., `UMH CA`

Create the server certificate by using the following commands (exchange mqtt.umh.app with your domain!):
```bash
/usr/share/easy-rsa/easyrsa gen-req mqtt.umh.app nopass
/usr/share/easy-rsa/easyrsa sign-req server mqtt.umh.app 

Copy the private key `pki/private/mqtt.umh.app.key` and the public certificate `pki/issued/mqtt.umh.app.crt` together with the root CA `pki/ca.crt` to the configuration of the MQTT broker.

## Adding new clients

Create new clients with following commands (remember to change TESTING with the planned MQTT client id): `export EASYRSA_REQ_CN=TESTING && /usr/share/easy-rsa/easyrsa gen-req $EASYRSA_REQ_CN nopass && /usr/share/easy-rsa/easyrsa sign-req client $EASYRSA_REQ_CN nopass`
