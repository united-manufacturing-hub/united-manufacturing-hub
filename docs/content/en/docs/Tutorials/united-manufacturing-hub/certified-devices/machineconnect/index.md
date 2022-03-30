---
title: "How to use machineconnect"
linkTitle: "How to use machineconnect"
weight: 1
aliases:
    - /docs/tutorials/certified-devices/machineconnect/
description: >
  This document explains how to install and use machineconnect
---

## Purpose

machineconnect is our certified device used for the connection of PLCs and installed in the switch cabinet of the machine. 

The idea behind machineconnect is to protect the PLC and all components with an additional firewall. Therefore, it is not accessible from outside of machineconnect except explicitly configured in the firewall. 

### Features

- Industrial Edge Computer 
    - With DIN rail mounting and 24V
    - Vibration resistent according to IEC 60068-2-27, IEC 60068-2-64/ MIL-STD-810, UNECE Reg.10 E-Mark, EN50155
    - Increased temperature range (-25°C ~ 70°C)
- Open source core installed and provisioned according to customer needs (e.g. MQTT certificates) in production mode (using k3OS)
- Additional security layer for your PLC by using [OPNsense](https://opnsense.org/) (incl. Firewall, Intrusion Detection, VPN)
- 10 years of remote VPN access via our servers included

## Physical installation

1. Attach wall mounting brackets to the chassis
2. Attach DIN Rail mounting brackets to the chassis
3. Clip system to the DIN Rail
4. Connect with 24V power supply
5. Connect Ethernet 1 with WAN / Internet
6. Connect Ethernet 3 with local switch (if existing). This connection will be called from now on "LAN".
7. (optional, see [connection to PLC](#connection-to-the-plc). If skipped please connect the PLC to Ethernet 3) Connect Ethernet 2 with PLC. This connection will be called from now on "PLC network". 

Verify the installation by turning on the power supply and checking whether all Ethernet LEDs are blinking.

## Connection to the PLC

There are two options to connect the PLC. We strongly recommend Option 1, but in some cases (PLC has fixed IP and is communicating with engine controllers or HMI and you cannot change the IP adresses there) you need to go for option 2.

### Option 1: The PLC gets the IP via DHCP (recommended)

1. Configure the PLC to retrieve the IP via DHCP
2. Configure OPNsense to give out the same IP for the MAC-address of the PLC for LAN. Go to Services --> DHCPv4 --> LAN  and add the PLC under "DHCP static mappings for this device" 

### Option 2: The PLC has a static IP, which cannot be changed

#### New method

> This method should be quicker

1. Add a new interface for the PLC network
2. Add a new route wit the target being the PLC IP and as gateway the automatically created gateway for the PLC (will not be shown by default, need to enter PLC_GW to be shown)
3. Change NAT to "Hybrid outbound NAT rule generation" and add a NAT for PLC, Source the LAN network, Destination the PLC
4. Thats it

#### Old method

1. Adding a new interface for the PLC network, e.g. "S7". {{< imgproc 1.png Fit "800x800" >}}{{< /imgproc >}}
2. Adding a new gateway (see screenshot. Assuming 192.168.1.150 is the IP of the PLC and the above created interface is called "S7") {{< imgproc 2.png Fit "800x800" >}}{{< /imgproc >}}
3. Adding a new route (see screenshot and assumptions of step 2) {{< imgproc 3.png Fit "800x800" >}}{{< /imgproc >}}
4. Changing NAT to "Manual outbound NAT rule generation" (see screenshot and assumptions of step 2) {{< imgproc 4.png Fit "800x800" >}}{{< /imgproc >}}
5. Add firewall rule to the PLC interface (see screenshot and assumptuons of step 2) {{< imgproc 5.png Fit "800x800" >}}{{< /imgproc >}}
6. Add firewall rule to LAN allowing interaction between LAN network and PLC network {{< imgproc 6.png Fit "800x800" >}}{{< /imgproc >}}

If you are struggling with these steps and have bought a machineconnect from us, feel free to contact us!

## Next steps

After doing the physical setup and connecting the PLC you can continue with [part 3 of the getting started guide](/docs/getting-started/connecting-machines-creating-dashboards).

