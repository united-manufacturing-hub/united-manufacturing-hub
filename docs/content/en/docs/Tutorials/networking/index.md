
---
title: "Edge networking"
linkTitle: "Edge networking"
weight: 4 
description: >
  The UMH stack features a sophisticated system to be integrated into any enterprise network. Additionally, it forces multiple barriers against attacks by design. This document should clear up any confusion.

---

## factorycube

The factorycube (featuring the RUT955) consists out of two separate networks:

1. internal
2. external

{{< imgproc networking Fit "1200x1200" >}}{{< /imgproc >}}

The internal network connects all locally connected machines, sensors and miniPCs with each other. The external network is "the connection to the internet". The internal network can access the external network, but not the other way around, except specifically setting firewall rules ("port forwarding").

### Example components in internal network

- Laptop for setting up
- Router
- miniPC
- ifm Gateways
- Ethernet Cameras

### Example components in external network

- Router (with its external IP)
- the "Internet" / server
