---
title: "3. Using it in production"
linkTitle: "3. Using it in production"
weight: 2
description: >
  This section explains how the system can be setup and run safely in production
---

We recommend adjusting the values.yaml to your needs. We recommend the following values for production:

## General

- In case of internet access: adding Cloudflare in front of the HTTP / HTTPS nodes (e.g. Grafana, factoryinsight) to provide an additional security layer
- We also recommend using LetsEncrypt (e.g. with cert-manager)

## TimescaleDB
- Setting timescaleDB replica to 3 and tuning or disabling it altogether and using timescaleDB Cloud (https://www.timescale.com/products)
- Adjusting resources for factoryinsight, enabling pdb and hpa and pointing it to the read replica of the timescaleDB database. We recommend pointing it to the read replica to increase performance and to prevent too much database load on the primary database.

## VerneMQ
- We recommend setting up a PKI infrastructure for MQTT (see also prerequisites) and adding the certs to `vernemq.CAcert` and following in the helm chart (by default there are highly insecure certificates there)
- You can adjust the ACL (access control list) by changing `vernemq.AclConfig`
- We highly recommend opening the unsecured port 1883 to the internet as everyone can connect there anonymously (`vernemq.service.mqtt.enabled` to `false`)
- If you are using the VerneMQ binaries in production you need to accept the verneMQ EULA (which disallows using it in production without contacting them)
- We recommend using 3 replicas on 3 different phyiscal servers for high availability setups

## Redis
- We recommend using 3 replicas
- The password is generated once during setup and stored in the secret redis-secret

## Nodered
- We recommend disabling external access to nodered entirely and spawning a seperate nodered instance for every project (to avoid having one node crashing all flows)
- You can change the configuration in `nodered.settings`
- We recommend that you set a password for accessing the webinterface in the `nodered.settings`

## Grafana
- We recommend two replicas on two seperate phyiscal server
- We also recommend changing the database password in `grafana.grafana.ini.database.password` (the database will automatically use this value during database setup)

## mqtt-to-postgresql
- We recommend at least two replicas on two seperate phyisical server
- It uses the same database access as factoryinsight. So if you want to switch it you can do it in factoryinsight

## factoryinsight
- We recommend at least two replicas on two seperate phyiscal server
- We strongly recommend that you change the passwords (they will automatically be used across the system, e.g. in the Grafana plugin)

