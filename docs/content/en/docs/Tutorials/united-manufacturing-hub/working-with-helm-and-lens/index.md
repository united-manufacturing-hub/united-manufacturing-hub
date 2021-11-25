---
title: "Working with Kubernetes, Helm and Lens"
linkTitle: "Working with Kubernetes, Helm and Lens"
description: >
    This article explains how to work with Kubernetes, Helm and Lens, especially how to enable / disable functionalities, update the configuration or how to do software upgrade 
---

## How does Kubernetes and Helm work?

> Where is my Dockerfile?

One might ask.

> Where is my docker-compose.yaml?

In this chapter, we want to explain how you can configure the Kubernetes cluster and therefore the United Manufacturing Hub. 

[![Kubernetes and Helm - how to work with them explained for an mechanical engineer](kubernetes-and-helm.png)](kubernetes-and-helm.png)

Informal and not exact explaination (from the right to the left):
Docker containers are called in Kubernetes lanugage Pods. 

There is also other stuff going on in Kubernetes like secret management or ingresses. 

You define the final state ("I want to have one application based on this Docker image with this environment variables ....") and Kubernetes will take care of the rest using its magic. This could be seen equal to a `docker-compose.yaml`

The final state is defined using Kubernetes object descriptions.

Example for a Kubernetes object description:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  selector:
    matchLabels:
      app: nginx
  replicas: 2 # tells deployment to run 2 pods matching the template
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
        ports:
        - containerPort: 80
```

You could now create and maintain all of them manually, which is what some companies do.

To do that automatically, you can use Helm.

Helm will take a values.yaml and will automatically create the corresponding Kubernetes object descriptions.

Example for a Helm template values:
```yaml
### mqttbridge ###

mqttbridge:
  enabled: true
  image: unitedmanufacturinghub/mqtt-bridge
  storageRequest: 1Gi

### barcodereader ###

barcodereader:
  enabled: true
  image: unitedmanufacturinghub/barcodereader
  customUSBName: "Datalogic ADC, Inc. Handheld Barcode Scanner"
  brokerURL: "factorycube-edge-emqxedge-headless"  # do not change, points to emqxedge i
  brokerPort: 1883  # do not change
  customerID: "raw"
  location: "barcodereader"
  machineID: "barcodereader"
```

There is nothing comparable to Helm in the Docker world.

For this to work, one needs to import a so-called Helm chart first. This is nothing else than a Kubernetes object description with some variables.

Example for a Kubernetes object description template (for Helm):
```yaml
{{- if .Values.mqttbridge.enabled -}}
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: {{ include "factorycube-edge.fullname" . }}-mqttbridge
  labels:
    {{- include "factorycube-edge.labels.mqttbridge" . | nindent 4 }}
spec:
  serviceName: {{ include "factorycube-edge.fullname" . }}-mqttbridge
  replicas: 1
  selector:
    matchLabels:
      {{- include "factorycube-edge.labels.mqttbridge" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "factorycube-edge.labels.mqttbridge" . | nindent 8 }}
    spec:
      containers:
      - name: {{ include "factorycube-edge.fullname" . }}-mqttbridge
        {{- if .Values.mqttbridge.tag }}
        image: {{ .Values.mqttbridge.image }}:{{ .Values.mqttbridge.tag }}
        {{- else }}
        image: {{ .Values.mqttbridge.image }}:{{ .Chart.AppVersion }}
        {{- end }}
```

A Helm chart can either be stored in a folder (e.g., when you clone the [United Manufacturing Hub repository](https://github.com/united-manufacturing-hub/united-manufacturing-hub)) or in a [Helm repository](https://helm.sh/docs/topics/chart_repository/) (which we use in our default installation script).

You might be tempted to just clone the repository and change stuff in the Kubernetes object description template (in the subfolder `templates`). We consider this a bad idea as these changes are now hard to track and to replicate somewhere else. Furthermore, when updating the Helm chart to a newer version of the United Manufacturing Hub these changes will be lost and overwritten.

In Lens you can now change the underlying Kubernetes object descriptions by e.g., clicking on a Deployment and then selecting edit. However, these changes will be reverted once you apply the Helm chart again.

Therefore, we recommend in production setups to only adjust the values.yaml in the Helm chart.

## Changing the configuration / updating values.yaml

There are two ways to change the `values.yaml`:

### using Lens GUI

Note: if you encounter the issue "path not found", please go to the [troubleshooting section](#troubleshooting) further down.

For this open Lens, select Apps on the left side and then click on Releases. A Release is a deployed version of a chart. Now you can not only change the values.yaml, but also update the Helm chart to a newer version.

### using CLI / kubectl in Lens

To override single entries in values.yaml you can use the `--set` command, for example like this:
`helm upgrade factorycube-edge . --namespace factorycube-edge --set nodered.env.NODE_RED_ENABLE_SAFE_MODE=true`

## Troubleshooting

### RELEASE (factorycube-edge or factorycube-server) not found when updating the Helm chart

There are some modifications needed to be done when using [Lens](https://k8slens.dev/) with Charts that are in a Repository. Otherwise, you might get a "RELEASE not found" message when changing the values in Lens.

The reason for this is, that Lens cannot find the source of the Helm chart. Although the Helm repository and the chart is installed on the Kubernetes server / edge device this does not mean that Lens can access it. You need to add the helm repository with the same name to your computer as well. 

So execute the following commands on your computer running Lens (on Windows in combination with WSL2):

1. Install Helm

```bash
export VERIFY_CHECKSUM=false && curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3  && chmod 700 get_helm.sh && ./get_helm.sh
```

2. Add the repository
```bash
helm repo add united-manufacturing-hub https://repo.umh.app
```
