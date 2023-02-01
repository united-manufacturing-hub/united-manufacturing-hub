#!/usr/bin/env bash

set -xe

sudo curl -sL https://github.com/kubernetes-sigs/kind/releases/download/v0.7.0/kind-Linux-amd64 -o /usr/local/bin/kind
sudo chmod 755 /usr/local/bin//kind

sudo curl -sL https://storage.googleapis.com/kubernetes-release/release/v1.17.4/bin/linux/amd64/kubectl -o /usr/local/bin/kubectl
#sudo chmod 755 /usr/local/bin//kubectl

curl -LO https://get.helm.sh/helm-v3.1.2-linux-amd64.tar.gz
tar -xzf helm-v3.1.2-linux-amd64.tar.gz
sudo mv linux-amd64/helm /usr/local/bin/
rm -rf helm-v3.1.2-linux-amd64.tar.gz

kind version
kubectl version --client=true
helm version

kind -q create cluster --wait 10m

kubectl get nodes
