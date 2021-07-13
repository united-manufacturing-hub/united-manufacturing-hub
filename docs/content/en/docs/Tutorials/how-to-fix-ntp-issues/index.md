---
title: "How to fix certificate not yet valid issues"
linkTitle: "How to fix certificate not yet valid issues"
description: >
  curl might fail and not download helm as the certificate is not yet valid. This happens especially when you are in a restricted network and the edge device is not able fetch the current date and time via NTP.  
---

While executing the command `export VERIFY_CHECKSUM=false && curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 && chmod 700 get_helm.sh && ./get_helm.sh` on k3OS you might get a error message like this:

```
curl: (60) SSL certificate problem: certificate is not yet valid
More details here: https://curl.se/docs/sslcerts.html

curl failed to verify the legitimacy of the server and therefore could not
establish a secure connection to it. To learn more about this situation and
how to fix it, please visit the web page mentioned above.
```

Checking the time with `date` results in a timestamp from 2017.

## Possible Solution

The time is not configured properly. It can happen that your NTP server is blocked (especially if you are inside a university network). 

You can verify that by entering `sudo ntpd -d -q -n -p 0.de.pool.ntp.org`. If you get a result like this, then it is definitely blocked:
```
ntpd: '0.de.pool.ntp.org' is 62.141.38.38
ntpd: sending query to 62.141.38.38
Alarm clock
```

We recommend using the NTP server from the local university or ask your system administrator. For the RWTH Aachen / FH Aachen you can use `sudo ntpd -d -q -n -p ntp1.rwth-aachen.de` as specified [here](https://help.itc.rwth-aachen.de/service/uvjppv3cuan8/)
