---
title: "How to connect with SSH"
linkTitle: "How to connect with SSH"
description: >
  This article explains how to connect with an edge device via SSH
---

## For Windows

We recommend MobaXTerm. **TODO**

## For Linux

For Linux you can typically use the inbuilt commands to connect with a device via SSH. Connect using the following command:

`ssh <username>@<IP>`, e.g., `ssh rancher@192.168.99.118`.

{{< imgproc SSH_linux_1.png Fit "800x500" >}}Connect via SSH{{< /imgproc >}}

There will be a warning saying that the authenticity of the host can't be established. Enter `yes` to continue with the connection.

{{< imgproc SSH_linux_2.png Fit "800x500" >}}Warning message: The authenticity of host 'xxx' can't be established.{{< /imgproc >}}

Enter the password and press enter. The default password of the auto setup will be `rancher`.

{{< imgproc SSH_linux_3.png Fit "800x500" >}}Successfully logged in via SSH{{< /imgproc >}}
