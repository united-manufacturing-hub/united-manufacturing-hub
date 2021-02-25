#!/bin/bash

echo "init_stack.sh: Sleeping 20 seconds"
sleep 20

# Install Helm

# TODO: check whether helm already exists
echo "Installing Helm..."
export VERIFY_CHECKSUM=false 
curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 
chmod 700 get_helm.sh && ./get_helm.sh
echo "Helm installed!"

# Download factorycube-helm
# TODO: replace with github link once published
echo "Get new version of factorycube-helm..."
wget http://172.21.9.175/factorycube-helm.tar.gz
tar -xvf factorycube-helm.tar.gz -C /home/rancher
rm factorycube-helm.tar.gz
echo "Got new version of factorycube-helm!"

# Creating MQTT secrets
echo "Creating MQTT secrets..."
kubectl create secret generic vernemq-certificates-secret --from-file=tls.crt=/home/rancher/ssl_certs/tls.crt --from-file=tls.key=/home/rancher/ssl_certs/tls.key --from-file=ca.crt=/home/rancher/ssl_certs/ca.crt
echo "MQTT secrets created!"

# Install factorycube-helm
echo "Installing factorycube-helm..."
helm install factorycube-helm /home/rancher/factorycube-helm --kubeconfig /etc/rancher/k3s/k3s.yaml
echo "factorycube-helm installed!"