---
title: "Fixing broken Node-RED flows"
linkTitle: "Fixing broken Node-RED flows"
description: >
  This tutorial shows how you can recover Node-RED flows that are stuck in an endless loop of crashing. 
---

## Prerequisites

- Node-RED in a crash loop because of one misconfigured node especially [azure-iot-hub](https://flows.nodered.org/node/node-red-contrib-azure-iot-hub) and [python3-function](https://flows.nodered.org/node/node-red-contrib-python3-function))
- Node-RED installed as part of the United Manufacturing Hub (either as factorycube-edge or factorycube-server)

## Tutorial

The solution is to boot Node-RED in safe mode by changing the environment variable `NODE_RED_ENABLE_SAFE_MODE` to `true`.

### After 0.6.1

TODO

### Before 0.6.1

1. Open Lens and connect to the cluster (you should know how to do it if you followed the [Getting Started guide](/docs/getting-started/setup-development/#step-3-connect-via-ssh))
2. Select the namespace, where Node-RED is in (factorycube-edge or factorycube-server)
3. Select the StatefulSet Node-RED. A popup should appear on the right side.
4. Press the edit button
{{< imgproc 1.png Fit "1280x500" >}}Press the edit button / pen symbol on top right of the screen{{< /imgproc >}}
5. Find the line 
```yaml
env:
    - name: TZ
      value: Berlin/Europe
```
and change it to this:
```yaml
env:
    - name: TZ
      value: Berlin/Europe
    - name: NODE_RED_ENABLE_SAFE_MODE
      value: "true"
```
6. Check whether it says `"true"` and not `true` (use quotation marks!)
7. Press Save
8. Terminate the pod manually if necessary (or if you are impatient)
9. Node-RED should now start in safe mode. This means that it will boot, but will not execute any flows.
10. Do your changes, fix the Nodes 
11. Do steps 1 - 5, but now set `NODE_RED_ENABLE_SAFE_MODE` to `false`

