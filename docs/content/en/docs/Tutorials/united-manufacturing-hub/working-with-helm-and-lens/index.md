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

TODO
