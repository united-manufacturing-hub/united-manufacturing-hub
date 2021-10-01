---
title: "How to work with MinIO"
linkTitle: "How to work with MinIO"
aliases:
    - /docs/tutorials/using-minio/
description: >
  This article explains how you can access MinIO for development and administration purposes
---

Please also take a look on our [guide on how to use it in production](/docs/getting-started/usage-in-production)!

## Default settings / development setup

By default MinIO is exposed outside of the Kubernetes cluster using a LoadBalancer. The default credentials are minio:minio123 for the console. When using `minikube` you can access the LoadBalancer service using `minikube tunnel` and then accessing the `external IP` using the selected port.
