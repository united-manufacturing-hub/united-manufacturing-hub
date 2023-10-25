#!/usr/bin/env bash

set -e
export INSTALL_K3S_VERSION=v1.28.2+k3s1
export INSTALL_KUBECTL_VERSION=v1.28.2
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

echo "[Step 0] Configuring SELinux"
if getenforce | grep -Fq 'Enforcing'; then
  sudo yum install -y container-selinux selinux-policy-base
  sudo update-crypto-policies --set DEFAULT:SHA1
  sudo yum install -y https://rpm.rancher.io/k3s/latest/common/centos/7/noarch/k3s-selinux-0.2-1.el7_8.noarch.rpm
  sudo update-crypto-policies --set DEFAULT
fi


if systemctl list-units --full --all | grep -Fq 'nm-cloud-setup.service'; then
  sudo systemctl disable --now nm-cloud-setup.service
fi


echo "[Step 1] Installing k3s"
sudo update-crypto-policies --set DEFAULT:SHA1
curl -fsSL -o /tmp/get_k3s.sh https://get.k3s.io
chmod +x /tmp/get_k3s.sh
sudo /tmp/get_k3s.sh
sudo update-crypto-policies --set DEFAULT

echo "[Step 2] Installing helm"
curl -fsSL -o /tmp/get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
chmod +x /tmp/get_helm.sh
/tmp/get_helm.sh

echo "[Step 3] Allow access to k3s.yaml"
sudo chmod 644 /etc/rancher/k3s/k3s.yaml

echo "[Step 4] Add UMH repo"
/usr/local/bin/helm repo add united-manufacturing-hub https://repo.umh.app/

echo "[Step 5] Add UMH namespace"
kubectl create namespace united-manufacturing-hub --kubeconfig /etc/rancher/k3s/k3s.yaml

echo "[Step 6] Install UMH"
/usr/local/bin/helm install united-manufacturing-hub united-manufacturing-hub/united-manufacturing-hub --set serialNumber=$(hostname) --kubeconfig /etc/rancher/k3s/k3s.yaml -n united-manufacturing-hub

