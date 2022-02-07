---
title: "3. Using it in production"
linkTitle: "3. Using it in production"
weight: 2
description: >
  This section explains how the system can be setup and run safely in production
---

This article is split up into two parts:

The first part will focus on factorycube-edge and the Industrial Automation world. The second part will focus on factorycube-server and the IT world.

## Using it on the edge

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

Our certified device "machineconnect" will have that network setup by default. Our certified device "factorycube" has a little bit different network setup, which you can take a look at [here](/docs/tutorials/general/networking).

### Other useful commands

{{< alert title="Note" color="info">}}
Deprecated! Please use installation script instead specified in development.yaml
{{< /alert >}}

Quick setup on k3OS:

1. `export VERIFY_CHECKSUM=false && curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3  && chmod 700 get_helm.sh && ./get_helm.sh`
2. `curl -L https://github.com/united-manufacturing-hub/united-manufacturing-hub/tarball/v0.4.2 | tar zx && mv $(find . -maxdepth 1  -type d -name "united-manufacturing-hub*") united-manufacturing-hub`
3. `helm install factorycube-edge /home/rancher/united-manufacturing-hub/deployment/factorycube-edge --values "/home/rancher/CUSTOM.yaml" --kubeconfig /etc/rancher/k3s/k3s.yaml`

## factorycube-server

{{< alert title="Note" color="info">}}
Deprecated! These changes need to be applied not for factorycube-server, but to the new Helm chart `united-manufacturing-hub`. It may require some additional changes. It is left in as it is still better than nothing.
{{< /alert >}}

In general the factorycube-server installation is tailored strongly to the environments it is running in. Therefore, we can only provide general guidance on setting it up.

**WARNING: THIS SECTION IS STILL IN WORK, PLEASE ONLY USE AS A ROUGH START. WE STRONGLY RECOMMEND CONTACTING US IF YOU ARE PLANNING ON USING IT IN PRODUCTION ENVIRONMENT AND WITH EXPOSURE TO THE INTERNET**

## Example deployment on AWS EKS

To give you an better idea, this section explains an example production deployment on AWS EKS.

### Preparation

#### General
- Use Cloudflare as DNS and firewall. This will provide an additional security layer on top of all your HTTP/HTTPS applications, e.g., factoryinsight or Grafana

#### AWS 

- Setup a AWS EKS cluster using [eksctl](https://docs.aws.amazon.com/eks/latest/userguide/getting-started-eksctl.html)
- Setup a S3 bucket and a IAM user
- Add IAM policy to the user (assuming the bucket is called `umhtimescaledbbackup`)
```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "s3:GetObject",
                "s3:PutObject",
                "s3:ListBucket"
            ],
            "Resource": [
                "arn:aws:s3:::umhtimescaledbbackup/*",
                "arn:aws:s3:::umhtimescaledbbackup"
            ]
        }
    ]
}
```

If you do not add the IAM policy you might get a `ACCESS DENIED` in `pgbackrest` pod.

#### Kubernetes

- Create a namespace called `dev2`
- Use later the release name `dev2`. If you are using a different release name, you might need to adjust `dev2` in the following `aws_eks.yaml` file
- Setup [nginx-ingress-controller](https://kubernetes.github.io/ingress-nginx/) (e.g., using the bitnami helm chart)
- Setup [external-dns](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/nginx-ingress.md)
- Setup [cert-manager](https://cert-manager.io/docs/tutorials/acme/ingress/) and create a certificate issuer called `letsencrypt-prod` (see also link)
- to enable backup using S3 buckets create a secret called `dev2-pgbackrest` and enter the following content:
```yaml
kind: Secret
apiVersion: v1
metadata:
  name: dev2-pgbackrest
  namespace: dev2
data:
  PGBACKREST_REPO1_S3_BUCKET: <redacted>
  PGBACKREST_REPO1_S3_ENDPOINT: <redacted>
  PGBACKREST_REPO1_S3_KEY: <redacted>
  PGBACKREST_REPO1_S3_KEY_SECRET: <redacted>
  PGBACKREST_REPO1_S3_REGION: <redacted>
type: Opaque

```
  
### aws_eks.yaml

We recommend the following values to get your journey to production started (using release name `dev2` and namespace `dev2`):

```yaml

### factoryinsight ###
factoryinsight:
  enabled: true
  image: unitedmanufacturinghub/factoryinsight
  replicas: 2
  redis:
    URI1: dev2-redis-node-0.dev2-redis-headless:26379
    URI2: dev2-redis-node-1.dev2-redis-headless:26379
    URI3: dev2-redis-node-2.dev2-redis-headless:26379
  db_host: "dev2-replica"
  db_port: "5433"
  db_password: "ADD_STRONG_PASSWORD_HERE"
  ingress:
    enabled: true
    publicHost: "api.dev2.umh.app"
    publicHostSecretName: "factoryinsight-tls-secret"
    annotations:
      external-dns.alpha.kubernetes.io/cloudflare-proxied: "false"
      cert-manager.io/cluster-issuer: "letsencrypt-prod" 
  resources:
    limits:
       cpu: 1000m         
    requests:
       cpu: 200m      


### mqtt-to-postresql ###
mqtttopostgresql:
  enabled: true
  image: unitedmanufacturinghub/mqtt-to-postgresql
  replicas: 2
  storageRequest: 1Gi

### timescaleDB ###
timescaledb-single:
  enabled: true
  replicaCount: 2
  
  image:
    # Image was built from
    # https://github.com/timescale/timescaledb-docker-ha
    repository: timescaledev/timescaledb-ha
    tag: pg12-ts2.0-latest
    pullPolicy: IfNotPresent 
  
  backup:
    enabled: true
  
  persistentVolumes:
    data:
      size: 20Gi 
    wal:
      enabled: true
      size: 5Gi
  
### grafana ###
grafana:
  enabled: true

  replicas: 2

  image:
    repository: grafana/grafana
    tag: 7.5.9
    sha: ""
    pullPolicy: IfNotPresent

  service:
    type: ClusterIP 

  ingress:
    enabled: true
    annotations: 
      kubernetes.io/ingress.class: nginx
      kubernetes.io/tls-acme: "true"
    labels: {}
    path: /

    # pathType is only for k8s > 1.19
    pathType: Prefix

    hosts:
      - dev2.umh.app 

    tls: []

  ## Pass the plugins you want installed as a list.
  ##
  plugins: 
      - grafana-worldmap-panel
      - grafana-piechart-panel
      - aceiot-svg-panel
      - grafana-worldmap-panel
      - natel-discrete-panel
      - isaozler-paretochart-panel
      - williamvenner-timepickerbuttons-panel
      - agenty-flowcharting-panel
      - marcusolsson-dynamictext-panel
      - factry-untimely-panel
      - cloudspout-button-panel 


  ## Grafana's primary configuration
  ## NOTE: values in map will be converted to ini format
  ## ref: http://docs.grafana.org/installation/configuration/
  ##
  grafana.ini:
    paths:
      data: /var/lib/grafana/data
      logs: /var/log/grafana
      plugins: /var/lib/grafana/plugins
      provisioning: /etc/grafana/provisioning
    analytics:
      check_for_updates: true
    log:
      mode: console
    grafana_net:
      url: https://grafana.net
    database:
      host: dev2 
      user: "grafana"
      name: "grafana"
      password: "ADD_ANOTHER_STRONG_PASSWORD_HERE"
      ssl_mode: require
      type: postgres

  ## Add a seperate remote image renderer deployment/service
  imageRenderer:
    # Enable the image-renderer deployment & service
    enabled: true
    replicas: 1

####################### nodered #######################
nodered:
  enabled: true 
  tag: 1.2.9
  port: 1880
  storageRequest: 1Gi
  timezone: Berlin/Europe
  serviceType: ClusterIP
  ingress:
    enabled: true
    publicHost: "nodered.dev2.umh.app"
    publicHostSecretName: "nodered-tls-secret"
    annotations:
      external-dns.alpha.kubernetes.io/cloudflare-proxied: "false"
      cert-manager.io/cluster-issuer: "letsencrypt-prod"
  settings: |-  
    module.exports = {
        // the tcp port that the Node-RED web server is listening on
        uiPort: process.env.PORT || 1880,
        // By default, the Node-RED UI accepts connections on all IPv4 interfaces.
        // To listen on all IPv6 addresses, set uiHost to "::",
        // The following property can be used to listen on a specific interface. For
        // example, the following would only allow connections from the local machine.
        //uiHost: "127.0.0.1",
        // Retry time in milliseconds for MQTT connections
        mqttReconnectTime: 15000,
        // Retry time in milliseconds for Serial port connections
        serialReconnectTime: 15000,
        // The following property can be used in place of 'httpAdminRoot' and 'httpNodeRoot',
        // to apply the same root to both parts.
        httpRoot: '/nodered',
        // If you installed the optional node-red-dashboard you can set it's path
        // relative to httpRoot
        ui: { path: "ui" },
        // Securing Node-RED
        // -----------------
        // To password protect the Node-RED editor and admin API, the following
        // property can be used. See http://nodered.org/docs/security.html for details.
        adminAuth: {
            type: "credentials",
            users: [
                {
                    username: "admin",
                    password: "ADD_NODERED_PASSWORD",
                    permissions: "*"
                }
            ]
        },
        
        functionGlobalContext: {
            // os:require('os'),
            // jfive:require("johnny-five"),
            // j5board:require("johnny-five").Board({repl:false})
        },
        // `global.keys()` returns a list of all properties set in global context.
        // This allows them to be displayed in the Context Sidebar within the editor.
        // In some circumstances it is not desirable to expose them to the editor. The
        // following property can be used to hide any property set in `functionGlobalContext`
        // from being list by `global.keys()`.
        // By default, the property is set to false to avoid accidental exposure of
        // their values. Setting this to true will cause the keys to be listed.
        exportGlobalContextKeys: false,
        // Configure the logging output
        logging: {
            // Only console logging is currently supported
            console: {
                // Level of logging to be recorded. Options are:
                // fatal - only those errors which make the application unusable should be recorded
                // error - record errors which are deemed fatal for a particular request + fatal errors
                // warn - record problems which are non fatal + errors + fatal errors
                // info - record information about the general running of the application + warn + error + fatal errors
                // debug - record information which is more verbose than info + info + warn + error + fatal errors
                // trace - record very detailed logging + debug + info + warn + error + fatal errors
                // off - turn off all logging (doesn't affect metrics or audit)
                level: "info",
                // Whether or not to include metric events in the log output
                metrics: false,
                // Whether or not to include audit events in the log output
                audit: false
            }
        },
        // Customising the editor
        editorTheme: {
            projects: {
                // To enable the Projects feature, set this value to true
                enabled: false
            }
        }
    }


##### CONFIG FOR REDIS #####
redis:
  enabled: true
  cluster:
    enabled: true
    slaveCount: 2
  image:
    pullPolicy: IfNotPresent
    registry: docker.io
    repository: bitnami/redis
    tag: 6.0.9-debian-10-r13
  master:
    extraFlags:
    - --maxmemory 4gb
    persistence:
      size: 8Gi
    resources:
      limits:
        memory: 4Gi
      requests:
        cpu: 100m
        memory: 1Gi
  podDisruptionBudget:
    enabled: true
    minAvailable: 2
  slave:
    persistence:
      size: 8Gi
    resources:
      limits:
        memory: 4Gi
      requests:
        cpu: 100m
        memory: 1Gi

##### CONFIG FOR VERNEMQ #####

vernemq:
  enabled: true
  AclConfig: |-
     pattern write ia/raw/%u/#
     pattern write ia/%u/#
     pattern $SYS/broker/connection/%c/state

     user TESTING
     topic ia/#
     topic $SYS/#
     topic read $share/TESTING/ia/#

     user ia_nodered
     topic ia/#
  CACert: |-
    ADD CERT
  Cert: |-
    ADD CERT
  Privkey: |-
    ADD CERT
  image:
    pullPolicy: IfNotPresent 
    repository: vernemq/vernemq
    tag: 1.11.0
  replicaCount: 2 
  service:
    annotations:
      prometheus.io/port: "8888"
      prometheus.io/scrape: "true"
      external-dns.alpha.kubernetes.io/cloudflare-proxied: "false"
      external-dns.alpha.kubernetes.io/hostname: mqtt.dev2.umh.app
    mqtts:
      enabled: true
      nodePort: 8883
      port: 8883
    mqtt:
      enabled: false
    type: LoadBalancer

```

### Further adjustments

#### VerneMQ / MQTT

- We recommend setting up a PKI infrastructure for MQTT (see also prerequisites) and adding the certs to `vernemq.CAcert` and following in the helm chart (by default there are highly insecure certificates there)
- You can adjust the ACL (access control list) by changing `vernemq.AclConfig`
- If you are using the VerneMQ binaries in production you need to accept the verneMQ EULA (which disallows using it in production without contacting them)

#### Redis
- The password is generated once during setup and stored in the secret redis-secret

#### Nodered
- We recommend disabling external access to nodered entirely and spawning a seperate nodered instance for every project (to avoid having one node crashing all flows)
- You can change the configuration in `nodered.settings`
- We recommend that you set a password for accessing the webinterface in the `nodered.settings`. See also [the official tutorial from nodered](https://nodered.org/docs/user-guide/runtime/securing-node-red#generating-the-password-hash)

#### MinIO

We strongly recommend to change all passwords and salts specified in values.yaml
