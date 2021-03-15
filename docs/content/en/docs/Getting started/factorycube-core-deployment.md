---
title: "Setting up a scalable deployment system"
linkTitle: "Setting up a scalable deployment system"
weight: 2
description: >
  This document describes how factorycube-core-deployment can be used and how it works.
---

## Prerequisites

You need a HTTP server running in your local network containing the configuration files. You can setup one with for example
`docker run -d -it -p 80:80 -v "C:\git\united-manufacturing-hub\deployment\factorycube-core-deployment:/usr/share/nginx/html:ro" nginx`

Package the helm chart with the following command:
`cd deployment && tar -czvf ./factorycube-core-deployment/factorycube-helm.tar.gz ./factorycube-core && cd ..`

## During installation

Now you can use the following cloud-init script during the installation of k3os: `http://YOUR_IP/configs/SERIAL_NUMBER.yaml`
