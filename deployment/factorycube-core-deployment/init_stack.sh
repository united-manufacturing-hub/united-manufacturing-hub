#!/bin/bash

WEBSERVER=172.21.9.175

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
wget http://$WEBSERVER/factorycube-helm.tar.gz
tar -xvf factorycube-helm.tar.gz -C /home/rancher
rm factorycube-helm.tar.gz
echo "Got new version of factorycube-helm!"

# Install factorycube-helm
echo "Installing factorycube-core..."
helm install factorycube-core /home/rancher/factorycube-core --values "/home/rancher/configs/factorycube-core-helm-values.yaml" --set serialNumber=$(hostname) --kubeconfig /etc/rancher/k3s/k3s.yaml
echo "factorycube-core installed!"