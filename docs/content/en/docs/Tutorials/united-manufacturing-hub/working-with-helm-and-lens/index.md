---
title: "Working with Helm and Lens"
linkTitle: "Working with Helm and Lens"
description: >
    This article explains how to work with Helm and Lens, especially how to update the configuration or how to do software upgrade 
---

## Changing the configuration / updating values.yaml

### using Lens GUI

Note: if you encounter the issue "not found", please go to the [troubleshooting section](#troubleshooting) further down.

### using CLI / kubectl in Lens

To override single entries in values.yaml you can use the `--set` command, for example like this:
`helm update factorycube-edge . --namespace factorycube-edge --set nodered.env.NODE_RED_ENABLE_SAFE_MODE=true`

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
