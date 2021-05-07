---
title: "How to update the stack / helm chart"
linkTitle: "How to update the stack / helm chart"
description: >
  This article explains how to update the helm chart, so that you can apply changes to the configuration of the stack or to install newer versions
---

## Prerequisites

none

## Tutorial

1. Go to the folder `deployment/factorycube-server` or `deployment/factorycube-edge`
2. Execute `helm upgrade factorycube-server . --values "YOUR VALUES FILE" --kubeconfig /etc/rancher/k3s/k3s.yaml -n YOUR_NAMESPACE`

This is basically your installation command, but you exchange install with upgrade. You need to change "YOUR VALUES FILE" with the path of your values.yaml, e.g. `/home/rancher/united-manufacturing-hub/deployment/factorycube-server/values.yaml` and you need to adjust  YOUR_NAMESPACE with the correct namespace name. If you did not specify any namespace during the installation you can use the namespace `default`. If you are using factorycube-edge instead of factorycube-server you need to adjust that as well. 
