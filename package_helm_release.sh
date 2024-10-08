#!/usr/bin/env bash

# Install Helm if not already installed
if ! command -v helm &> /dev/null; then
  echo "Helm is not installed. Installing Helm..."
  curl -fsSL -o /tmp/get_helm.sh https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3
  chmod 700 /tmp/get_helm.sh
  /tmp/get_helm.sh
fi

# Check if the user has set the required environment variables
# VERSION, REPO_URL
if [ -z "$VERSION" ]; then
  echo "VERSION is not set. Please set the VERSION environment variable."
  exit 1
fi

if [ -z "$REPO_URL" ]; then
  # Default to https://management.umh.app/helm
  REPO_URL="https://management.umh.app/helm"
fi

echo "Packaging Helm chart with version $VERSION"

# Update version & appVersion of the Helm chart
CHART_PATH="./deployment/united-manufacturing-hub/"
REPO_PATH="./deployment/helm-repo"

# Check if paths exist
if [ ! -d "$CHART_PATH" ]; then
  echo "Helm chart path $CHART_PATH does not exist"
  exit 1
fi

if [ ! -d "$REPO_PATH" ]; then
  echo "Helm repo path $REPO_PATH does not exist"
  exit 1
fi

# Update version & appVersion in Chart.yaml
sed -i "s/version:.*/version: $VERSION/g" $CHART_PATH/Chart.yaml
sed -i "s/appVersion:.*/appVersion: $VERSION/g" $CHART_PATH/Chart.yaml

CURRENT_DIR=$(pwd)
# Package the Helm chart
cd $REPO_PATH
helm package "../united-manufacturing-hub"

# Index the Helm chart
helm repo index --url $REPO_URL --merge index.yaml .

cd $CURRENT_DIR

echo "Helm chart packaged successfully"

git add deployment
git commit -m "Bump Helm chart version to $VERSION"
git push