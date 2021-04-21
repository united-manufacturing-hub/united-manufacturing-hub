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

Please check out the notes for [using it in development installations](#notes-for-development-installations) and for [using it in production environments](#notes-for-production-environments)

1. Clone repository (in the future: add helm repo)
2. Configure values.yaml according to your needs. For help in configuring you can take a look into the respective documentation of the subcharts ([Grafana](https://github.com/grafana/helm-charts), [redis](https://github.com/bitnami/charts/tree/master/bitnami/redis), [timescaleDB](https://github.com/timescale/timescaledb-kubernetes/tree/master/charts/timescaledb-single), [verneMQ](https://github.com/vernemq/docker-vernemq/tree/master/helm/vernemq)) or into the documentation of the subcomponents ([factoryinsight](../../developers/factorycube-server/factoryinsight), [mqtt-to-postgresql](../../developers/factorycube-server/mqtt-to-postgresql))
3. Execute `helm install factorycube-server .` and wait. Helm will automatically install the entire stack across multiple node. It can take up to several minutes until everything is setup. 

Everything should be now successfully setup and you can connect your edge devices and start creating dashboards! **Keep in mind**: the default values.yaml should only be used for development environments and never for production. See also notes below.

## Using it

You can now access Grafana and nodered via HTTP / HTTPS (depending on your setup). Default user for Grafana is admin. You can find the password in the secret RELEASE-NAME-grafana. Grafana is available via port 8080, nodered via 1880. 

If you are fine with a non-production development setup we recommend that you skip the following part and go directly to [part 3](../connecting-machines-creating-dashboards)

## Notes for development installations

When installing factorycube-server on the same edge device as factorycube-edge we recommend the following adjustments:

1. Install factorycube-edge according to [Getting Started part 1](../factorycube-edge)
    - Install it in a namespace called `factorycube-edge` with `kubectl create namespace factorycube-edge` and `helm install ... -n factorycube-edge`. 
    - When setting up the certificates use the certificates in `factorycube-server/developmentCertificates/pki/` and then `ca.crt`, `issued/TESTING.crt` and `issued/private/TESTING.key`. 
    - Additionally use as `mqttBridgeURL` `ssl://factorycube-server-vernemq-local-service.factorycube-server:8883`. 
    - You can also use the following example [`development_values.yaml`](/examples/factorycube-server/development_values.yaml). However, you still need to adjust the iprange from sensorconnect in case you want to use it.
2. Install `factorycube-server` in the namespace `factorycube-server` (see also 1.)

## Notes for production environments

We recommend adjusting the values.yaml to your needs. We recommend the following values for production:

### General

- In case of internet access: adding Cloudflare in front of the HTTP / HTTPS nodes (e.g. Grafana, factoryinsight) to provide an additional security layer
- We also recommend using LetsEncrypt (e.g. with cert-manager)

### TimescaleDB
- Setting timescaleDB replica to 3 and tuning or disabling it altogether and using timescaleDB Cloud (https://www.timescale.com/products)
- Adjusting resources for factoryinsight, enabling pdb and hpa and pointing it to the read replica of the timescaleDB database. We recommend pointing it to the read replica to increase performance and to prevent too much database load on the primary database.

### VerneMQ
- We recommend setting up a PKI infrastructure for MQTT (see also prerequisites) and adding the certs to `vernemq.CAcert` and following in the helm chart (by default there are highly insecure certificates there)
- You can adjust the ACL (access control list) by changing `vernemq.AclConfig`
- We highly recommend opening the unsecured port 1883 to the internet as everyone can connect there anonymously (`vernemq.service.mqtt.enabled` to `false`)
- If you are using the VerneMQ binaries in production you need to accept the verneMQ EULA (which disallows using it in production without contacting them)
- We recommend using 3 replicas on 3 different phyiscal servers for high availability setups

### Redis
- We recommend using 3 replicas
- The password is generated once during setup and stored in the secret redis-secret

### Nodered
- We recommend disabling external access to nodered entirely and spawning a seperate nodered instance for every project (to avoid having one node crashing all flows)
- You can change the configuration in `nodered.settings`
- We recommend that you set a password for accessing the webinterface in the `nodered.settings`

### Grafana
- We recommend two replicas on two seperate phyiscal server
- We also recommend changing the database password in `grafana.grafana.ini.database.password` (the database will automatically use this value during database setup)

### mqtt-to-postgresql
- We recommend at least two replicas on two seperate phyisical server
- It uses the same database access as factoryinsight. So if you want to switch it you can do it in factoryinsight

### factoryinsight
- We recommend at least two replicas on two seperate phyiscal server
- We strongly recommend that you change the passwords (they will automatically be used across the system, e.g. in the Grafana plugin)

