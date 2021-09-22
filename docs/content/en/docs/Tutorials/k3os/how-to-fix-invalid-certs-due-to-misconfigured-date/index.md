---
title: "How to fix certificate not yet valid issues"
linkTitle: "How to fix certificate not yet valid issues"
description: >
  curl might fail and not download helm as the certificate is not yet valid. This happens especially when you are in a restricted network and the edge device is not able fetch the current date and time via NTP.  
---



## Issue 1

While executing the command `export VERIFY_CHECKSUM=false && curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 && chmod 700 get_helm.sh && ./get_helm.sh` on k3OS you might get a error message like this:

```
curl: (60) SSL certificate problem: certificate is not yet valid
More details here: https://curl.se/docs/sslcerts.html

curl failed to verify the legitimacy of the server and therefore could not
establish a secure connection to it. To learn more about this situation and
how to fix it, please visit the web page mentioned above.
```

Checking the time with `date` results in a timestamp from 2017. There are two possible solutions.

### Possible solution 1: configure NTP

The time is not configured properly. It can happen that your NTP server is blocked (especially if you are inside a university network). 

You can verify that by entering `sudo ntpd -d -q -n -p 0.de.pool.ntp.org`. If you get a result like this, then it is definitely blocked:
```
ntpd: '0.de.pool.ntp.org' is 62.141.38.38
ntpd: sending query to 62.141.38.38
Alarm clock
```

We recommend using the NTP server from the local university or ask your system administrator. For the RWTH Aachen / FH Aachen you can use `sudo ntpd -d -q -n -p ntp1.rwth-aachen.de` as specified [here](https://help.itc.rwth-aachen.de/service/uvjppv3cuan8/)

### Possible solution 2: set hardware clock via BIOS

Go into the BIOS and set the hardware clock of the device manually.

## Issue 2

k3os reports errors, due hardware date being in the past. (ex: 01.01.2017). Example: during startup the k3s certificates are generated, however, it is still using the hardware time. Even after setting the time manually with NTP it wont let you connect with k3s as the certificates created during startup are not not valid anymore. Setting the time is not persisted during reboots.

### Steps to Reproduce

1. Install k3os, without updating BIOS clock
2. Install UMH
3. helm will fail on **Install factorycube-server** step, due to outdated certificates.

### Possible solution

Load cloudinit with added ntp_servers on OS install. You can use the one at https://www.umh.app/development.yaml

Be careful: you need to host it on a HTTP server (not HTTPS) as you would get other certificate issues while fetching it.

