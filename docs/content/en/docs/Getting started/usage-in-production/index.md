---
title: "3. Using it in production"
linkTitle: "3. Using it in production"
weight: 2
description: >
  This section explains how the system can be setup and run safely in production
---

This article is split up into two parts:

The first part will focus on factorycube-edge and the Industrial Automation world. The second part will focus on factorycube-server and the IT world.

## factorycube-edge

The world of Industrial Automation is heavily regulated as very often not only expensive machines are controlled, but also machines that can potentially injure a human being. Here are some information that will help you in setting it up in production (not legal advice!).

If you are unsure about how to setup something like this, you can contact us for help with implementation and/or certified devices, which will ease the setup process! 

### Hardware & Installation, Reliability

One key component in Industrial Automation is reliability. Hardware needs to be carefully selected according to your needs and standards in your country.

When changing things at the machine, you need to ensure that you are not voiding the warranty or to void the CE certification of the machine. Even just installing something in the electrical rack and/or connecting with the PLC can do that! And it is not just unnecessary regulations, it is actually important:

PLCs can be pretty old and usually do not have much capacity for IT applications. Therefore, it is essential when extracting data to not overload the PLCs capabilities by requesting too much data. We strongly recommend to test the performance and closely watch the CPU and RAM usage of the PLC.

This is the reason we install sometimes additional sensors instead of plugging into the existing ones. And sometimes this is enough to get the relevant KPIs out of the machine, e.g., the Overall Equipment Effectiveness (OEE).

### Network setup

To ensure the safety of your network and PLC we recommend a network setup like following:

{{< imgproc networking Fit "1200x1200" >}}Network setup having the machines network, the internal network and PLC network seperated from each other{{< /imgproc >}}

The reason we recommend this setup is to ensure security and reliability of the PLC and to follow industry best-practices, e.g. the ["Leitfaden Industrie 4.0 Security"](https://industrie40.vdma.org/documents/4214230/15280277/1492501068630_Leitfaden_I40_Security_DE.pdf/836f1356-12e6-4a00-9a4d-e4bcc07101b4) from the VDMA (Verband Deutscher Maschinenbauer) or [Rockwell](https://literature.rockwellautomation.com/idc/groups/literature/documents/wp/enet-wp038_-en-p.pdf). 

Additionally, we are taking more advanced steps than actually recommended (e.g., preventing almost all network traffic to the PLC) as we have seen very often, that machine PLC are usually not setup according to best-practices and manuals of the PLC manufacturer by system integrators or even the machine manufacturer due to a lack of knowledge. Default passwords not changed or ports not closed, which results in unnecessary attack surfaces. 

Also updates are almost never installed on a machine PLC resulting in well-known security holes to be in the machines for years.

Another argument is a pretty practical one: In Industry 4.0 we see more and more devices being installed at the shopfloor and requiring access to the machine data. Our stack will not be the only one accessing and processing data from the production machine. There might be entirely different solutions out there, who need real-time access to the PLC data. Unfortunately, a lot of these devices are proprietary and sometimes even with hidden remote access features (very common in Industrial IoT startups unfortunately...). 
We created the additional DMZ around each machine to prevent one solution / hostile device at one machine being able to access the entire machine park. There is only one component (usually node-red) communicating with the PLC and sending the data to MQTT. If there is one hostile device somewhere it will have very limited access by default except specified otherwise, as it can get all required data directly from the MQTT data stream.

Our certified device "machineconnect" will have that network setup by default. Our certified device "factorycube" has a little bit different network setup, which you can take a look at [here](../../tutorials/networking).

## factorycube-server

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

