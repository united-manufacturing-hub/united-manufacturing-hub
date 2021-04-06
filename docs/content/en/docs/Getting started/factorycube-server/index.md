---
title: "2. Setting up factorycube-server"
linkTitle: "2. Setting up factorycube-server"
weight: 2
description: >
  This section explains how factorycube-server can be installed on the servers 
---

## Prerequisites

- Kubernetes is setup and helm is installed (you can also use Kubernetes-as-a-service providers like GKE, AKS, etc.)
- (production-only) Setup PKI infrastructure for MQTT according to [this guide](../../tutorials/pki)

## Installation steps

1. Clone repository (in the future: add helm repo)
2. Configure values.yaml. For a one-node devepment setup you can use [values-development.yaml](TODO)
3. Execute `helm install factorycube-server .` and wait. Helm will automatically install the entire stack across multiple node. It can take up to several minutes until everything is setup. 

Everything should be now successfully setup and you can connect your edge devices and start creating dashboards!

## Notes for production environments

We recommend adjusting the values.yaml to your needs. We recommend the following values for production:

- In case of internet access: adding Cloudflare in front of the HTTP / HTTPS nodes (e.g. Grafana, factoryinsight) to provide an additional security layer
- Setting timescaleDB replica to 3 and tuning or disabling it altogether and using timescaleDB Cloud (https://www.timescale.com/products)
- Adjusting resources for factoryinsight, enabling pdb and hpa and pointing it to the read replica of the timescaleDB database. We recommend pointing it to the read replica to increase performance and to prevent too much database load on the primary database.
