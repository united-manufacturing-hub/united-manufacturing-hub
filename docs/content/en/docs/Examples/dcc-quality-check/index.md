---
title: "Quality monitoring in a bottling line at the Digital Capability Center Atlanta"
linkTitle: "Quality monitoring"
weight: 3
description: >
  This document describes the quality monitoring use case
---

## Profile

At the Digital Capability Center (DCC) Atlanta, an Industry 4.0 learning and demonstration factory, a bottling line for filling water bottles was retrofitted with an artificial intelligence quality inspection system. With the help of a camera connected to an ia: factorycube, the bottles are checked for quality defects and sorted out by a pneumatic device in the event of a defect.

### Photos of the machines

{{< imgproc picture1 Fit "500x300" >}}{{< /imgproc >}}
{{< imgproc picture2 Fit "500x300" >}}{{< /imgproc >}}
{{< imgproc picture3 Fit "500x300" >}}{{< /imgproc >}}

## Challenges

### Manual visual inspection causes high costs

- Each individual bottle is checked for quality defects by an employee
- One employee is assigned to each shift exclusively for quality inspection

### Customer complaints and claims due to undetected quality defects

- Various quality defects are difficult to detect with the naked eye and are occasionally overlooked

### No data on quality defects that occur for product and process improvement

- Type and frequency of quality defects are not recorded and documented
- No data exists that can be analyzed to derive improvement measures for product and process optimization

## Solution

### Integration

TODO: #67 Add integration for DCC quality check

### Installed hardware

#### factorycube

![](/images/products/factorycube.png)

A machine learning model runs on the factorycube, which evaluates and classifies the images. See also [factorycube].

#### Gateways

![](/images/products/gateway.png)

Gateways connect the sensors to the factorycube.

Models:

- ifm AL1352

#### Light barriers

![](/images/products/lightbarrier_1.png)

A light barrier identifies the bottle and sends a signal to the factorycube to trigger the camera.

Models:

- ifm O5D100 (Optical distance sensor)

#### Camera

![](/images/products/camera.jpg)

A camera takes a picture of the bottle and sends it to the factorycube.

Models:

- Allied Vision (Mako G-223)

### Detectable quality defects

{{< imgproc DIAGRAM Fit "500x300" >}}{{< /imgproc >}}

### Automated action

As soon as a quality defect is detected the defect bottle is automatically kicked out by the machine.
