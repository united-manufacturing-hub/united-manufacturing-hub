CHART_PATH := deployment/united-manufacturing-hub
HELM_REPO_PATH := deployment/helm-repo
CHART_NAME := united-manufacturing-hub
VERSION := $(shell grep '^version:' $(CHART_PATH)/Chart.yaml | awk '{print $2}')
BRANCH_NAME := $(shell git rev-parse --abbrev-ref HEAD | sed 's/\./-/g')

.PHONY: all lint template package index release package-prerelease index-prerelease release-prerelease

# Lint the Helm chart
lint:
	helm lint $(CHART_PATH)

# Template the Helm chart for debugging (using default values.yaml)
template:
	helm template $(CHART_NAME) $(CHART_PATH) --debug --values $(CHART_PATH)/values.yaml > $(CHART_PATH)/rendered.yaml

# Package the Helm chart (create the .tgz)
package:
	helm package $(CHART_PATH) -d $(HELM_REPO_PATH)

# Update the index.yaml in the default Helm repository
index:
	helm repo index $(HELM_REPO_PATH) --url https://management.umh.app/helm/umh

# Create a new release (package and update index.yaml in default repo)
release: lint package index
	git add $(HELM_REPO_PATH)/*.tgz $(HELM_REPO_PATH)/index.yaml
	git commit -m "Release $(VERSION)"
	git push

# Package the Helm chart for prerelease
package-prerelease:
	helm package $(CHART_PATH) -d $(HELM_REPO_PATH)

# Update the index.yaml in the prerelease Helm repository
index-prerelease:
	helm repo index $(HELM_REPO_PATH) --url https://$(BRANCH_NAME).united-manufacturing-hub.pages.dev

# Create a new prerelease (package and update index.yaml in prerelease repo)
release-prerelease: lint package-prerelease index-prerelease
	git add $(HELM_REPO_PATH)/*.tgz $(HELM_REPO_PATH)/index.yaml
	git commit -m "Prerelease $(VERSION) - $(BRANCH_NAME)"
	git push

update-depedencies:
	helm dependency update $(CHART_PATH)

update-go-dependencies:
	@echo "Updating Go dependencies..."
	@SHELL=/bin/bash; \
	go work sync || true; \
	go work vendor || true; \
	currentFolder=$$(pwd); \
	cd golang; \
	go get -u ./...; \
	go mod tidy; \
	go work sync || true; \
	cd $$currentFolder; \
	go work sync || true; \
	go work vendor || true; \