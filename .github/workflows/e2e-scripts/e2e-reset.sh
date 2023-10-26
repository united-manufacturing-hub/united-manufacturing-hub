#!/usr/bin/env bash

# Echo statements added to describe each step
echo "[Step 1] Initializing script to remove k3s, Helm, and kubectl"

# Set script options and environment variables
set -e
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

# Automated k3s uninstall
echo "[Step 2] Running the automated k3s uninstall script"
sudo /usr/local/bin/k3s-uninstall.sh || true

# Delete specific namespaces
echo "[Step 3] Deleting specific namespaces"
sudo kubectl delete namespace united-manufacturing-hub || true
sudo kubectl delete namespace mgmtcompanion || true

# Stop k3s service
echo "[Step 4] Stopping k3s services"
sudo systemctl stop k3s || true
sudo pkill -f "/usr/local/bin/k3s" || true

# Remove k3s and related components
echo "[Step 5] Removing k3s binaries and services"
sudo rm -rf "$(which k3s)" || true
sudo rm -rf /etc/systemd/system/k3s.service || true
sudo rm -rf /etc/systemd/system/k3s*.service || true
sudo systemctl daemon-reload || true

echo "[Step 6] Removing k3s files and directories"
sudo rm -rf /etc/rancher/k3s || true
sudo rm -rf /run/k3s || true
sudo rm -rf /run/flannel || true
sudo rm -rf /var/lib/rancher/k3s || true
sudo rm -rf /var/lib/kubelet || true

# Remove Helm
echo "[Step 7] Removing Helm repo"
helm repo remove united-manufacturing-hub || true
sudo helm repo remove united-manufacturing-hub || true

echo "[Step 8] Removing Helm binary"
sudo rm -rf "$(which helm)" || true

# Remove kubectl
echo "[Step 9] Removing kubectl binary"
sudo rm -rf "$(which kubectl)" || true

# Uninstall SELinux packages
echo "[Step 10] Uninstalling SELinux packages"
sudo yum remove -y container-selinux selinux-policy-base || true

echo "[Step 11] Removing k3s SELinux policy"
sudo yum remove -y https://rpm.rancher.io/k3s/latest/common/centos/7/noarch/k3s-selinux-0.2-1.el7_8.noarch.rpm || true

# Remove SELinux modules
echo "[Step 12] Removing k3s SELinux modules"
sudo semodule -r k3s || true

# Remove dangling semodules
echo "[Step 13] Removing dangling semodules"
modules_to_delete=$(sudo semodule --list=full | grep k3s | awk '{print $1}')
if [ -n "$modules_to_delete" ]; then
  sudo semodule -X "$modules_to_delete" -r k3s || true
else
  echo "Nothing to delete."
fi

# Additional Cleanup
echo "[Step 14] Additional Cleanup"

# Remove user-specific kube config
echo "  [Step 14.1] Removing user-specific kube config"
rm -rf ~/.kube || true

# Clean up orphaned packages
echo "  [Step 14.2] Cleaning up orphaned packages"
sudo yum autoremove -y || true

# Remove previously downloaded content
sudo rm /tmp/install.sh || true
sudo rm /tmp/*.yaml || true

sleep 5

echo "[Step 15] k3s, Helm, and kubectl have been removed from the system"