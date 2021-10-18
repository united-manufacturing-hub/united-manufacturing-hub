---
title: "How to connect with SSH"
linkTitle: "How to connect with SSH"
aliases:
    - /docs/tutorials/connect-with-ssh/
description: >
  This article explains how to connect with an edge device via SSH
---

## For Windows

For Windows we recommend **MobaXTerm**. 

Get the **free** Version of MobaXTerm on https://mobaxterm.mobatek.net/download.html

{{< imgproc SSH_windows_1.png Fit "800x500" >}}MobaXTerm Session{{< /imgproc>}}

After launching the program, open a new session by clicking on "Session" in the upper left corner.

Enter your IP, e.g. `192.168.1.XXX`, in the Remote Host field. Select "Specify username". The specific username is `rancher`.

{{< imgproc SSH_windows_2.png Fit "800x500" >}}Password{{< /imgproc>}}

Enter the password and press enter. The password is `rancher`. You do not need to save the password, just click `No`.

{{< imgproc SSH_windows_3.png Fit "800x500" >}}Successfully logged in via SSH{{< /imgproc>}}

## For Linux

For Linux you can typically use the inbuilt commands to connect with a device via SSH. Connect using the following command:

`ssh <username>@<IP>`, e.g., `ssh rancher@192.168.99.118`.

{{< imgproc SSH_linux_1.png Fit "800x500" >}}Connect via SSH{{< /imgproc >}}

There will be a warning saying that the authenticity of the host can't be established. Enter `yes` to continue with the connection.

{{< imgproc SSH_linux_2.png Fit "800x500" >}}Warning message: The authenticity of host 'xxx' can't be established.{{< /imgproc >}}

Enter the password and press enter. The default password of the auto setup will be `rancher`.

{{< imgproc SSH_linux_3.png Fit "800x500" >}}Successfully logged in via SSH{{< /imgproc >}}
