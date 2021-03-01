# Installing the core stack

## Contents

- [Installing the core stack](#installing-the-core-stack)
  - [Contents](#contents)
  - [Method 1: using factorycube-core-deployment](#method-1-using-factorycube-core-deployment)
  - [Method 2: doing it on your own](#method-2-doing-it-on-your-own)

## Method 1: using factorycube-core-deployment

You can deploy the entire core with Helm and Kubernetes using `factorycube-core-deployment`. See also [here](factorycube-core-deployment.md).

## Method 2: doing it on your own

The following steps assume that you have a edge device running Kubernetes and Helm.

1. Copy the helm chart to the folder `/home/rancher/factorycube-core` (you can use other folders as well, but please adjust the folder name in the following code snippets)
2. Configure `factorycube-core` by creating a values.yaml file in the folder `/home/rancher/configs/factorycube-core-helm-values.yaml`, which you use to override values. Required: `mqttBridgeURL`, `mqttBridgeTopic`, `sensorconnect.iprange`, `mqttBridgeCACert`, `mqttBridgeCert`, `mqttBridgePrivkey`
3. Execute `helm install factorycube-core /home/rancher/factorycube-core --values "/home/rancher/configs/factorycube-core-helm-values.yaml" --set serialNumber=$(hostname) --kubeconfig /etc/rancher/k3s/k3s.yaml` (change kubeconfig and serialNumber accordingly)
