# Copyright 2025 UMH Systems GmbH
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

CHART_PATH := deployment/united-manufacturing-hub
HELM_REPO_PATH := deployment/helm-repo
CHART_NAME := united-manufacturing-hub
VERSION := $(shell grep '^version:' $(CHART_PATH)/Chart.yaml | awk '{print $2}')
BRANCH_NAME := $(shell git rev-parse --abbrev-ref HEAD | sed 's/\./-/g')

.PHONY: all lint template package index release package-prerelease index-prerelease release-prerelease

# Lint the Helm chart
helm-lint:
	helm lint $(CHART_PATH)

# Template the Helm chart for debugging (using default values.yaml)
helm-template:
	helm template $(CHART_NAME) $(CHART_PATH) --debug --values $(CHART_PATH)/values.yaml > $(CHART_PATH)/rendered.yaml

# Package the Helm chart (create the .tgz)
helm-package:
	helm package $(CHART_PATH) -d $(HELM_REPO_PATH)

# Update the index.yaml in the default Helm repository
helm-index:
	helm repo index $(HELM_REPO_PATH) --url https://management.umh.app/helm/umh

# Create a new release (package and update index.yaml in default repo)
helm-release: helm-lint helm-package helm-index
	git add $(HELM_REPO_PATH)/*.tgz $(HELM_REPO_PATH)/index.yaml
	git commit -m "Release $(VERSION)"
	git push

# Package the Helm chart for prerelease
helm-package-prerelease:
	helm package $(CHART_PATH) -d $(HELM_REPO_PATH)

# Update the index.yaml in the prerelease Helm repository
helm-index-prerelease:
	helm repo index $(HELM_REPO_PATH) --url https://$(BRANCH_NAME).united-manufacturing-hub.pages.dev

# Create a new prerelease (package and update index.yaml in prerelease repo)
helm-release-prerelease: helm-lint helm-package-prerelease helm-index-prerelease
	git add $(HELM_REPO_PATH)/*.tgz $(HELM_REPO_PATH)/index.yaml
	git commit -m "Prerelease $(VERSION) - $(BRANCH_NAME)"
	git push

helm-update-dependencies:
	helm dependency update $(CHART_PATH)


install-git-hooks:
	@echo "Installing git hooks..."
	go install github.com/evilmartians/lefthook@latest
	lefthook install

snyk:
	snyk test ./umh-core
