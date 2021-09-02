#!/bin/sh
(kubectl delete namespace factorycube-server && kubectl create namespace factorycube-server) &
#(kubectl delete namespace factorycube-edge && kubectl create namespace factorycube-edge) &

wait
helm install factorycube-server /home/rancher/united-manufacturing-hub/deployment/factorycube-server --values "/home/rancher/united-manufacturing-hub/deployment/factorycube-server/values.yaml" --kubeconfig /etc/rancher/k3s/k3s.yaml -n factorycube-server &
#helm install factorycube-edge /home/rancher/united-manufacturing-hub/deployment/factorycube-edge --values "/home/rancher/united-manufacturing-hub/deployment/factorycube-edge/development_values.yaml" --set serialNumber=$(hostname) --kubeconfig /etc/rancher/k3s/k3s.yaml -n factorycube-edge &
wait
