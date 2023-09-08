# Copyright 2023 UMH Systems GmbH
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
MAKEFLAGS += --no-builtin-rules

##@ Variables

## Print the help message for the target if set to y
PRINT_HELP=n
## Name of the k3d cluster
CLUSTER_NAME=umh
## Helm chart to install. Can be a local path or a remote chart
CHART=united-manufacturing-hub/united-manufacturing-hub
## Path to the Helm values file
VALUES_FILE=
## Space-separated list of Helm values to pass to the chart, in the form <key>=<value>. Omit the --set flag
VALUES=
## Helm chart version
VERSION=
## Container repository
CTR_REPO=ghcr.io/united-manufacturing-hub
## Container tag
CTR_TAG=latest
## Container image
CTR_IMG=barcodereader factoryinput factoryinsight grafana-proxy grafana-umh hivemq-init kafka-bridge kafka-debug kafka-init kafka-state-detector kafka-to-postgresql metrics mqtt-bridge mqtt-kafka-bridge sensorconnect
## Space-separated list of workloads. Each workload is a string in the form <type>:<name>
WORKLOADS=sts:kafka svc:kafka deploy:factoryinsight-deployment deploy:opcuasimulator-deployment deploy:iotsensorsmqtt sts:hivemqce sts:nodered deploy:grafanaproxy sts:mqttbridge sts:sensorconnect
## Space-separated list of flags to pass to go test
TEST_FLAGS=
## Space-separated list of directories
DIRS=./...
## Space-separated list of build flags to pass to docker buildx, in the form <key>=<value>
BUILD_FLAGS=

# Local variables. Do not override them
db_p:=$(shell python -c 'import socket; s=socket.socket(); s.bind(("", 0)); print(s.getsockname()[1])')
mqtt_p:=$(shell python -c 'import socket; s=socket.socket(); s.bind(("", 0)); print(s.getsockname()[1])')
node_red_p:=$(shell python -c 'import socket; s=socket.socket(); s.bind(("", 0)); print(s.getsockname()[1])')
opc_ua_p:=$(shell python -c 'import socket; s=socket.socket(); s.bind(("", 0)); print(s.getsockname()[1])')
gf_p:=$(shell python -c 'import socket; s=socket.socket(); s.bind(("", 0)); print(s.getsockname()[1])')
kafka_p:=$(shell python -c 'import socket; s=socket.socket(); s.bind(("", 0)); print(s.getsockname()[1])')
console_p:=$(shell python -c 'import socket; s=socket.socket(); s.bind(("", 0)); print(s.getsockname()[1])')
set_flags=$(if $(VALUES),$(addprefix --set ,$(VALUES)),)
values_flag=$(if $(VALUES_FILE),-f $(VALUES_FILE),)
version_flag=$(if $(VERSION),--version $(VERSION),)

##@ Cluster management

define CL_CR_HELP
# Create a k3d cluster with the given name and map the ports to the host machine
#
# Args:
#   CLUSTER_NAME: Name of the k3d cluster
#
# Example:
#   make cluster-create
#   make cluster-create CLUSTER_NAME=my-cluster
endef
.PHONY: cluster-create
ifeq ($(PRINT_HELP),y)
cluster-create:
	$(CL_CR_HELP)
else
## Create a k3d cluster with the given name and map the ports to the host machine
cluster-create:
	k3d cluster create $(CLUSTER_NAME) -p $(gf_p):8080@server:0 -p $(node_red_p):1880@server:0 -p $(db_p):5432@server:0 -p $(opc_ua_p):46010@server:0 -p $(mqtt_p):1883@server:0 -p $(kafka_p):9092@server:0 -p $(console_p):8090@server:0
endif

define CL_CL_HELP
# Delete the k3d cluster with the given name
#
# Args:
#   CLUSTER_NAME: Name of the k3d cluster
#
# Example:
#   make cluster-clean
#   make cluster-clean CLUSTER_NAME=my-cluster
endef
.PHONY: cluster-clean
ifeq ($(PRINT_HELP),y)
cluster-clean:
	$(CL_CL_HELP)
else
## Delete the cluster with the given name
cluster-clean:
	k3d cluster delete $(CLUSTER_NAME)
endif

define CL_IN_HELP
# Delete and recreate a k3d cluster and install an Helm chart.
# By default it installs the latest version of UMH in a cluster named umh with the default values.
#
# Args:
#   CLUSTER_NAME: Name of the k3d cluster
#   CHART: Helm chart to install. Can be a local path or a remote chart. Defaults to the UMH repository chart
#   VALUES: Space-separated list of Helm values to pass to the chart, in the form <key>=<value>. Omit the --set flag
#   VALUES_FILE: Path to the Helm values file to use. Use default values if not specified
#   VERSION: Helm chart version. Use the latest version if not specified
#
# Example:
#   make cluster-install
#   make cluster-install CLUSTER_NAME=my-cluster
#   make cluster-install CHART=./deployment/united-manufacturing-hub
#   make cluster-install VERSION=1.0.0
#   make cluster-install VALUES_FILE=./deployment/united-manufacturing-hub/values.yaml
#   make cluster-install VALUES="global.image.repository=ghcr.io/united-manufacturing-hub"
endef
.PHONY: cluster-install
ifeq ($(PRINT_HELP),y)
cluster-install:
	$(CL_IN_HELP)
else
## Recreate a k3d cluster and install the latest version of UMH
cluster-install: cluster-clean cluster-create helm-repo-update helm-install print-ports
endif

##@ Helm

.PHONY: helm-repo-update
## Update the Helm repositories
helm-repo-update:
	helm repo update

define HL_IN_HELP
# Install or upgrade the UMH release in the current cluster with the given chart, values file and version.
# By default it installs the latest version of UMH with the default values.
#
# Args:
#   CHART: Helm chart to install. Can be a local path or a remote chart. Defaults to the UMH repository chart
#   VALUES: Space-separated list of Helm values to pass to the chart, in the form <key>=<value>. Omit the --set flag
#   VALUES_FILE: Path to the Helm values file to use. Use default values if not specified
#   VERSION: Helm chart version. Use the latest version if not specified
#
# Example:
#   make helm-install
#   make helm-install CHART=./deployment/united-manufacturing-hub
#   make helm-install VERSION=1.0.0
#   make helm-install VALUES_FILE=./deployment/united-manufacturing-hub/values.yaml
#   make helm-install VALUES="global.image.tag=1.0.0 global.image.pullPolicy=Always"
endef
.PHONY: helm-install
ifeq ($(PRINT_HELP),y)
helm-install:
	$(HL_IN_HELP)
else
## Install or upgrade the UMH release in the current cluster with the given chart, values file and version
helm-install:
	helm upgrade united-manufacturing-hub $(CHART) -n united-manufacturing-hub --install --create-namespace $(values_flag) $(version_flag) $(set_flags) --cleanup-on-fail
endif

define HL_T_UP_HELP
# Create a cluster with the latest version of UMH and upgrade it to the local version
#
# Args:
#   CLUSTER_NAME: Name of the k3d cluster
#
# Example:
#   make helm-test-upgrade
#   make helm-test-upgrade CLUSTER_NAME=my-cluster
endef
.PHONY: helm-test-upgrade
ifeq ($(PRINT_HELP),y)
helm-test-upgrade:
	$(HL_T_UP_HELP)
else
## Create a cluster with the latest version of UMH and upgrade it to the local version
helm-test-upgrade: CLUSTER_NAME=umh-upgrade
helm-test-upgrade: cluster-clean cluster-create helm-repo-update helm-install kube-wait kube-clear-workloads print-ports
	@${MAKE} -s helm-install CHART=./deployment/united-manufacturing-hub
endif

define HL_T_UP_DT_HELP
# Create a cluster with the latest version of UMH and upgrade it to the local version, then run the data flow test Job
#
# Args:
#   CLUSTER_NAME: Name of the k3d cluster
#
# Example:
#   make helm-test-upgrade-with-data
#   make helm-test-upgrade-with-data CLUSTER_NAME=my-cluster
endef
.PHONY: helm-test-upgrade-with-data
ifeq ($(PRINT_HELP),y)
helm-test-upgrade-with-data:
	$(HL_T_UP_DT_HELP)
else
## Create a cluster with the latest version of UMH and upgrade it to the local version, then run the data flow test Job
helm-test-upgrade-with-data: CLUSTER_NAME=umh-upgrade
helm-test-upgrade-with-data: helm-test-upgrade
	@${MAKE} -s kube-wait
	@${MAKE} -s kube-test-data-flow-job
endif

define HL_T_LOC_CT_HELP
# Build the containers locally, create a cluster and install the latest version of UMH with the local containers
#
# Args:
#   CLUSTER_NAME: Name of the k3d cluster. Defaults to umh-local-containers
#   CTR_TAG: Tag to use for the local containers. Defaults to helm-test-local-containers
#   VALUES: Space-separated list of Helm values to pass to the chart, in the form <key>=<value>. Omit the --set flag.
#           By default sets all the containers to use the same tag as the one passed in CTR_TAG
#
# Example:
#   make helm-test-local-containers
#   make helm-test-local-containers CLUSTER_NAME=my-cluster
endef
.PHONY: helm-test-local-containers
ifeq ($(PRINT_HELP),y)
helm-test-local-containers:
	$(HL_T_LOC_CT_HELP)
else
## Build the containers locally, create a cluster and install the latest version of UMH with the local containers
helm-test-local-containers: CLUSTER_NAME=umh-local-containers
helm-test-local-containers: CTR_TAG=helm-test-local-containers
helm-test-local-containers: VALUES=kafkastatedetector.image.repository=$(CTR_REPO)/kafkastatedetector kafkastatedetector.image.tag=$(CTR_TAG) mqttbridge.image=$(CTR_REPO)/mqttbridge mqttbridge.tag=$(CTR_TAG) barcodereader.image.repository=$(CTR_REPO)/barcodereader barcodereader.image.tag=$(CTR_TAG) sensorconnect.image=$(CTR_REPO)/sensorconnect sensorconnect.tag=$(CTR_TAG) kafkabridge.image.repository=$(CTR_REPO)/kafkabridge kafkabridge.image.tag=$(CTR_TAG) kafkabridge.initContainer.repository=$(CTR_REPO)/kafka-init kafkabridge.initContainer.tag=$(CTR_TAG) factoryinsight.image.repository=$(CTR_REPO)/factoryinsight factoryinsight.image.tag=$(CTR_TAG) factoryinput.image.repository=$(CTR_REPO)/factoryinput factoryinput.image.tag=$(CTR_TAG) grafanaproxy.image.repository=$(CTR_REPO)/grafana-proxy grafanaproxy.image.tag=$(CTR_TAG) kafkatopostgresql.image.repository=$(CTR_REPO)/kafka-to-postgresql kafkatopostgresql.image.tag=$(CTR_TAG) kafkatopostgresql.initContainer.repository=$(CTR_REPO)/kafka-init kafkatopostgresql.initContainer.tag=$(CTR_TAG) tulipconnector.image.repository=$(CTR_REPO)/tulip-connector tulipconnector.image.tag=$(CTR_TAG) mqttkafkabridge.image.repository=$(CTR_REPO)/mqtt-kafka-bridge mqttkafkabridge.image.tag=$(CTR_TAG) mqttkafkabridge.initContainer.repository=$(CTR_REPO)/kafka-init mqttkafkabridge.initContainer.tag=$(CTR_TAG) metrics.image.repository=$(CTR_REPO)/metrics metrics.image.tag=$(CTR_TAG)
helm-test-local-containers: docker cluster-clean cluster-create helm-repo-update helm-install print-ports
endif

##@ Kubectl

define KB_CL_HELP
# Delete workloads in the current cluster. Useful when upgrading the UMH release.
#
# Args:
#   WORKLOADS: List of workloads to delete. Each workload is a string in the form <type>:<name>
#              The type can be the abbreviation of a Kubernetes resource type (e.g. "deploy", "svc", "sts", etc.)
#              The name is the name of the workload to delete, without the release name prefix.
#
# Example:
#   make kube-clear-workloads
#   make kube-clear-workloads WORKLOADS="deploy:iotsensorsmqtt sts:nodered"
endef
.PHONY: kube-clear-workloads
ifeq ($(PRINT_HELP),y)
kube-clear-workloads:
	$(KB_CL_HELP)
else
## Delete workloads in the current cluster. Useful when upgrading the UMH release
kube-clear-workloads:
	@echo "Clearing workloads..."
	$(foreach workload,$(WORKLOADS), type=$(firstword $(subst :, ,$(workload))); name=$(lastword $(subst :, ,$(workload))); kubectl delete $${type} united-manufacturing-hub-$${name} -n united-manufacturing-hub || true;)
endif

.PHONY: kube-wait
## Wait for all the pods in the UMH namespace to be ready
kube-wait:
	@echo "Waiting for UMH to be installed..."
	kubectl wait --for=condition=ready --timeout=-1s pods --all -n united-manufacturing-hub

.PHONY: kube-test-data-flow-job
## Run the data flow test Job
kube-test-data-flow-job:
	kubectl apply -f ./test/generic-data-flow-test-script.yaml -n united-manufacturing-hub
	kubectl apply -f ./test/data-flow-test/data-flow-test-job.yaml -n united-manufacturing-hub

##@ Docker

define DC_B_P_HELP
# Build and push the docker image for the given component.
# By default it builds and pushes all the images.
#
# Args:
#   CTR_IMG: Space-separated list of images to build and push. Defaults to all the images.
#   CTR_REPO: Docker repository to use. Defaults to "ghcr.io/united-manufacturing-hub"
#   CTR_TAG: Docker tag to use. Defaults to "latest"
#   BUILD_FLAGS: Flags to pass to the docker build command
#
# Example:
#   make docker
#   make docker CTR_IMG="barcodereader factoryinput"
#   make docker CTR_REPO="my-repo" CTR_TAG="1.0.0"
#   make docker BUILD_FLAGS="--no-cache"
endef
.PHONY: docker
ifeq ($(PRINT_HELP),y)
docker:
	$(DC_B_P_HELP)
else
## Build and push the docker images
docker: docker-build docker-push
endif

define DC_B_HELP
# Build the docker image for the given component.
# By default it builds all the images.
#
# Args:
#   CTR_IMG: Space-separated list of images to build. Defaults to all the images.
#   CTR_REPO: Docker repository to use. Defaults to "ghcr.io/united-manufacturing-hub"
#   CTR_TAG: Docker tag to use. Defaults to "latest"
#   BUILD_FLAGS: Flags to pass to the docker build command
#
# Example:
#   make docker-build
#   make docker-build CTR_IMG="barcodereader factoryinput"
#   make docker-build CTR_REPO="my-repo" CTR_TAG="1.0.0"
#   make docker-build BUILD_FLAGS="--platform linux/amd64"
endef
.PHONY: docker-build
ifeq ($(PRINT_HELP),y)
docker-build:
	$(DC_B_HELP)
else
## Build the docker images
docker-build: $(addsuffix -build,$(CTR_IMG))
$(addsuffix -build,$(CTR_IMG)):
	docker buildx build -t $(CTR_REPO)/$(subst -build,,$@):$(CTR_TAG) $(BUILD_FLAGS) -f ./deployment/$(subst -build,,$@)/Dockerfile .
endif

define DC_P_HELP
# Push the docker image for the given component.
# By default it pushes all the images.
#
# Args:
#   CTR_IMG: Space-separated list of images to push. Defaults to all the images.
#   CTR_REPO: Docker repository to use. Defaults to "ghcr.io/united-manufacturing-hub"
#   CTR_TAG: Docker tag to use. Defaults to "latest"
#
# Example:
#   make docker-push
#   make docker-push CTR_IMG="barcodereader factoryinput"
#   make docker-push CTR_REPO="my-repo" CTR_TAG="1.0.0"
endef
.PHONY: docker-push
ifeq ($(PRINT_HELP),y)
docker-push:
	$(DC_P_HELP)
else
## Push the docker images
docker-push: $(addsuffix -push,$(CTR_IMG))
$(addsuffix -push,$(CTR_IMG)):
	docker push $(CTR_REPO)/$(subst -push,,$@):$(CTR_TAG)
endif

##@ Go

.ONESHELL: go-deps
.PHONY: go-deps
## Download and install the dependencies
go-deps:
	@cd golang
	go get -t -d -v ./...
	go install ./...

define GO_T_HELP
# Run the unit tests.
#
# Args:
#   DIRS: Space-separated list of directories to test. Defaults to all the directories.
#   TEST_FLAGS: Flags to pass to the go test command.
#
# Example:
#   make go-test-unit
#   make go-test-unit DIRS="cmd/kafka-state-detector"
endef
.ONESHELL: go-test-unit
.PHONY: go-test-unit
ifeq ($(PRINT_HELP),y)
go-test-unit:
	$(GO_T_HELP)
else
## Run the unit tests
go-test-unit: go-deps
	@cd golang
	go test $(TEST_FLAGS) $(DIRS)
endif

##@ Utilities

.PHONY: help
## Display the help
help:
	@awk 'BEGIN { FS = "##"; printf "Usage|make [target] [VAR=VALUE]\n\n" } /^##[^@]/ { c=substr($$0,3); next }\
 		  c && /^[[:upper:]_]+=/ { printf "  \033[36m%s|\033[0m %s. Default %s\n", substr($$1,1,index($$1,"=")-1), c, substr($$1,index($$1,"=")+1,length); c=0 }\
 		  /^##[^@]/ { c=substr($$0,3); next }\
 		  c && /^[[:alnum:]-]+:/ { printf "  \033[36m%s|\033[0m %s\n", substr($$1,1,index($$1,":")-1), c; c=0 }\
 		  /^##@/ { printf "\n\033[32m%s|\033[0m \n", substr($$0, 5) } '\
 		  $(MAKEFILE_LIST) | column -s '|' -t

.PHONY: print-ports
## Print the ports of the services. Only useful as a prerequisite of other targets
print-ports:
	@echo "Ports:"
	@echo "Grafana: http://localhost:$(gf_p)"
	@echo "Node-RED: http://localhost:$(node_red_p)/nodered"
	@echo "PostgreSQL: http://localhost:$(db_p)"
	@echo "OPC UA Simulator: http://localhost:$(opc_ua_p)"
	@echo "MQTT Broker: http://localhost:$(mqtt_p)"
	@echo "Kafka: http://localhost:$(kafka_p)"
	@echo "Console: http://localhost:$(console_p)"
