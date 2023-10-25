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
# Add /usr/local/bin to path
export PATH=$PATH:/usr/local/bin/
/tmp/get_helm.sh

echo "[Step 3] Allow access to k3s.yaml"
sudo chmod 644 /etc/rancher/k3s/k3s.yaml

echo "[Step 4] Add UMH repo"
helm repo add united-manufacturing-hub https://repo.umh.app/

echo "[Step 5] Add UMH namespace"
kubectl create namespace united-manufacturing-hub --kubeconfig /etc/rancher/k3s/k3s.yaml

echo "[Step 6] Install UMH"
helm install united-manufacturing-hub united-manufacturing-hub/united-manufacturing-hub --set serialNumber=$(hostname) --kubeconfig /etc/rancher/k3s/k3s.yaml -n united-manufacturing-hub


echo "[Step 7] Install kubectl"
curl -LO "https://dl.k8s.io/release/$INSTALL_KUBECTL_VERSION/bin/linux/amd64/kubectl" -o /usr/local/bin/kubectl
sudo chmod +x kubectl

kubectl version

kubectl get nodes

timeout=120  # 2 minutes
interval=5  # Check every 5 seconds
success=false

echo "[Step 8] Await UMH"

while (( timeout > 0 )); do
  if kubectl get pods --all-namespaces -o custom-columns='NAMESPACE:.metadata.namespace,NAME:.metadata.name,STATUS:.status.phase' --no-headers | awk '$3!="Running" && $3!="Succeeded" {exit 1}'; then
    printf "\tPods have started\n"
    success=true
    break
  fi
  failedCount=$(kubectl get pods --all-namespaces -o custom-columns='NAMESPACE:.metadata.namespace,NAME:.metadata.name,STATUS:.status.phase' --no-headers | awk '$3!="Running" && $3!="Succeeded"' | wc -l)
  totalCount=$(kubectl get pods --all-namespaces -o custom-columns='NAMESPACE:.metadata.namespace,NAME:.metadata.name,STATUS:.status.phase' --no-headers | wc -l)
  printf "\f%s/%s failed pods, retrying\n" "$failedCount" "$totalCount"

  sleep $interval
  timeout=$((timeout - interval))
done

if [ "$success" = true ]; then
  printf "\tAll pods are Running or Succeeded.\n"
  # Continue with the rest of the script
else
  printf "\tPods failed to startup in time"
  # Print pod list and exit with code 1
  kubectl get pods --all-namespaces
  exit 1
fi