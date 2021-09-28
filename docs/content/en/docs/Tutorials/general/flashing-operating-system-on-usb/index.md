---
title: "How to flash an operating system onto a USB-stick"
linkTitle: "How to flash an operating system onto a USB-stick"
aliases:
    - /docs/tutorials/flashing-operating-system-on-usb/
description: >
  There are multiple ways to flash a operating system onto a USB-stick. We will present you the method of using [balenaEtcher](https://www.balena.io/etcher/).
---

### Prerequisites

- You need a USB-stick (we recommend USB 3.0 for better speed)
- You need a OS image in the *.iso format. For k3OS you could choose for example [this version](https://github.com/rancher/k3os/releases/download/v0.20.7-k3s1r0/k3os-amd64.iso)

### Steps

{{< imgproc balena_1.png Fit "800x500" >}}Download balenaEtcher: www.balena.io/etcher/ {{< /imgproc >}}
{{< imgproc balena_2.png Fit "800x500" >}}Insert USB-stick and open balenaEtcher{{< /imgproc >}}
{{< imgproc balena_3.png Fit "800x500" >}}Select downloaded *.iso by clicking on "Flash from file" (the sceeen might look different based on your operating system){{< /imgproc >}}
{{< imgproc balena_4.png Fit "800x500" >}}Select the USB-stick by clicking on "Select target"{{< /imgproc >}}
{{< imgproc balena_5.png Fit "800x500" >}}Select "Flash"{{< /imgproc >}}
{{< imgproc balena_6.png Fit "800x500" >}}It will flash the image on the USB-stick{{< /imgproc >}}
{{< imgproc balena_7.png Fit "800x500" >}}You are done!{{< /imgproc >}}

These steps are also available as a YouTube tutorial from the user kilObit.
{{< youtube kWRx40Q8B_A >}}
