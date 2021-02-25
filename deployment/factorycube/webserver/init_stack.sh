#!/bin/bash

# Install Helm

# TODO: check whether helm already exists
export VERIFY_CHECKSUM=false 
curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 
chmod 700 get_helm.sh && ./get_helm.sh

# Remove previous version
helm uninstall factorycube-helm --kubeconfig /etc/rancher/k3s/k3s.yaml
rm -R /home/rancher/factorycube-helm

# Download factorycube-helm
# TODO: replace with github link once published
wget http://172.21.9.175/factorycube-helm.tar.gz
tar -xvf factorycube-helm.tar.gz -C /home/rancher
rm factorycube-helm.tar.gz

# Install factorycube-helm
# TODO: currently failing first time as /etc/rancher/k3s/k3s.yaml does not exist yet
helm install factorycube-helm /home/rancher/factorycube-helm --kubeconfig /etc/rancher/k3s/k3s.yaml