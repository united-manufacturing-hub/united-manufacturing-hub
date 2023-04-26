.PHONY: all

#################
### VARIABLES ###
#################

# Evnironment variables. You can override them by setting them in the command line.
CLUSTER_NAME=umh # Name of the cluster
CHART=united-manufacturing-hub/united-manufacturing-hub # Chart to install
VALUES_FILE= # Values file to use
VERSION= # Version to install
CTR_REPO=ghcr.io/united-manufacturing-hub # Container repository to use
CTR_TAG=latest # Container tag to use
CTR_IMG=barcodereader \
factoryinput \
factoryinsight \
grafana-proxy \
grafana-umh \
hivemq-init \
kafka-bridge \
kafka-debug \
kafka-init \
kafka-state-detector \
kafka-to-postgresql \
metrics \
mqtt-bridge \
mqtt-kafka-bridge \
sensorconnect

# Local variables. Do not override them.
db_p:=$(shell python -c 'import socket; s=socket.socket(); s.bind(("", 0)); print(s.getsockname()[1])')
mqtt_p:=$(shell python -c 'import socket; s=socket.socket(); s.bind(("", 0)); print(s.getsockname()[1])')
node_red_p:=$(shell python -c 'import socket; s=socket.socket(); s.bind(("", 0)); print(s.getsockname()[1])')
opc_ua_p:=$(shell python -c 'import socket; s=socket.socket(); s.bind(("", 0)); print(s.getsockname()[1])')
gf_p:=$(shell python -c 'import socket; s=socket.socket(); s.bind(("", 0)); print(s.getsockname()[1])')
kafka_p:=$(shell python -c 'import socket; s=socket.socket(); s.bind(("", 0)); print(s.getsockname()[1])')
console_p:=$(shell python -c 'import socket; s=socket.socket(); s.bind(("", 0)); print(s.getsockname()[1])')
values_flag=$(if $(VALUES_FILE),-f $(VALUES_FILE),)
version_flag=$(if $(VERSION),--version $(VERSION),)

##########################
### CLUSTER MANAGEMENT ###
##########################

# Create a k3d cluster with the given name and map the ports to the host machine
create-cluster:
	@echo "Creating cluster..."
	k3d cluster create $(CLUSTER_NAME) -p $(gf_p):8080@server:0 -p $(node_red_p):1880@server:0 -p $(db_p):5432@server:0 -p $(opc_ua_p):46010@server:0 -p $(mqtt_p):1883@server:0 -p $(kafka_p):9092@server:0 -p $(console_p):8090@server:0

# Delete the cluster with the given name
clean-cluster:
	@echo "Cleaning cluster..."
	k3d cluster delete $(CLUSTER_NAME)

######################
### HELM & KUBECTL ###
######################

# Update the Helm repositories
helm-repo-update:
	helm repo update

# Install or upgrade the UMH release in the current cluster with the given chart, values file and version.
# If no values file is given, the default values are used. If no version is given, the latest version is installed.
# If no chart is given, the latest version is installed.
install:
	@echo "Installing UMH..."
	helm upgrade united-manufacturing-hub $(CHART) -n united-manufacturing-hub --install --create-namespace $(values_flag) $(version_flag) --cleanup-on-fail

# Install the latest version of UMH in a dedicated cluster. It is possible to override the version and values file.
install-latest: CLUSTER_NAME=umh-latest
install-latest: CHART=united-manufacturing-hub/united-manufacturing-hub
install-latest: clean-cluster create-cluster helm-repo-update install print-ports

# Install the local version of UMH in a dedicated cluster. It is possible to override the values file.
install-current: CLUSTER_NAME=umh-current
install-current: CHART=./deployment/united-manufacturing-hub
install-current: clean-cluster create-cluster install print-ports

# Delete the workloads to upgrade the release
clean-workloads:
	@echo "Cleaning workloads..."
#	kubectl delete deployment united-manufacturing-hub-factoryinsight-deployment -n united-manufacturing-hub || true
#	kubectl delete deployment united-manufacturing-hub-iotsensorsmqtt -n united-manufacturing-hub || true
#	kubectl delete deployment united-manufacturing-hub-kafkatopostgresql -n united-manufacturing-hub || true
#	kubectl delete deployment united-manufacturing-hub-mqttkafkabridge -n united-manufacturing-hub || true
#	kubectl delete deployment united-manufacturing-hub-opcuasimulator-deployment -n united-manufacturing-hub || true
#	kubectl delete statefulset united-manufacturing-hub-nodered -n united-manufacturing-hub || true
#	kubectl delete statefulset united-manufacturing-hub-hivemqce -n united-manufacturing-hub || true
	kubectl delete statefulset united-manufacturing-hub-kafka -n united-manufacturing-hub || true
	kubectl delete service united-manufacturing-hub-kafka -n united-manufacturing-hub || true

# Run the data flow test Job
data-flow-test:
	@echo "Testing UMH..."
	kubectl apply -f ./test/generic-data-flow-test-script.yaml -n united-manufacturing-hub
	kubectl apply -f ./test/data-flow-test/data-flow-test-job.yaml -n united-manufacturing-hub

# Wait for UMH to be installed
wait-install:
	@echo "Waiting for UMH to be installed..."
	kubectl wait --for=condition=ready --timeout=-1s pods --all -n united-manufacturing-hub

# Test the upgrade from the latest version to the local version
test-helm-upgrade: CLUSTER_NAME=umh-upgrade
test-helm-upgrade: clean-cluster create-cluster helm-repo-update install wait-install clean-workloads print-ports
	@${MAKE} -s install CHART=./deployment/united-manufacturing-hub

# Test the upgrade from the latest version to the local version and run the data flow test
test-helm-upgrade-with-data: CLUSTER_NAME=umh-upgrade
test-helm-upgrade-with-data: clean-cluster create-cluster helm-repo-update install wait-install
	@${MAKE} -s install CHART=./deployment/united-manufacturing-hub
	@${MAKE} -s wait-install
	@${MAKE} -s data-flow-test
	@${MAKE} -s print-ports

##############
### DOCKER ###
##############

# Build and push the docker images
docker: $(CTR_IMG)
$(CTR_IMG): %:
	@echo "Building $*..."
	docker build -t $(CTR_REPO)/$*:$(CTR_TAG) -f ./deployment/$*/Dockerfile .
	docker push $(CTR_REPO)/$*:$(CTR_TAG)

###############
### GOLANG ###
###############

# Get the dependencies
go-deps:
	@echo "Getting dependencies..."
	@cd golang && go get -t -d -v ./...

# Run unit tests
test-go-unit: deps
	@echo "Running unit tests..."
	@cd golang && go test -v ./...

#################
### UTILITIES ###
#################

# Display the help
help:
	@echo "Usage: make [target] [VAR=VALUE]"
	@echo ""
	@echo "Targets for k3d management:"
	@echo "  create-cluster           Create a k3d cluster with the given name and map the ports to the host machine"
	@echo "  clean-cluster            Delete the cluster with the given name"
	@echo ""
	@echo "Targets for Helm and kubectl:"
	@echo "  helm-repo-update              Update the Helm repositories"
	@echo "  install                       Install or upgrade the UMH release in the current cluster with the given chart, values file and version"
	@echo "  install-latest                Install the latest version of UMH in a dedicated cluster"
	@echo "  install-current               Install the local version of UMH in a dedicated cluster"
	@echo "  clean-workloads               Delete the workloads to upgrade the release"
	@echo "  data-flow-test                Run the data flow test Job"
	@echo "  wait-install                  Wait for UMH to be installed"
	@echo "  test-helm-upgrade             Test the upgrade from the latest version to the local version"
	@echo "  test-helm-upgrade-with-data   Test the upgrade from the latest version to the local version and run the data flow test"
	@echo ""
	@echo "Targets for Golang:"
	@echo "  deps                     Get the dependencies"
	@echo "  test-go-unit             Run unit tests"
	@echo ""
	@echo "Targets for Docker:"
	@echo "  docker                   Build and push the docker images"
	@echo ""
	@echo "Environment variables:"
	@echo "  CLUSTER_NAME   The name of the cluster. Default: umh"
	@echo "  CHART          The chart to install. Default: united-manufacturing-hub/united-manufacturing-hub"
	@echo "  VALUES_FILE    The values file to use. Default: none (use the default values)"
	@echo "  VERSION        The version to install. Default: none (use the latest version)"
	@echo "  CTR_REPO       The container repository. Specify the registry if different than docker.io Default: ghcr.io/united-manufacturing-hub"
	@echo "  CTR_TAG        The container tag. Default: latest"
	@echo "  CTR_IMG        The container image. Can be a space separated list of names. Default to all the images in the deployment folder"
	@echo ""
	@echo "Examples:"
	@echo "  Install version 0.9.12 of UMH with the given values file:"
	@echo "  make install-latest VERSION=0.9.12 VALUES_FILE=./test/test-values-full.yaml"
	@echo ""
	@echo "  Install version 0.9.12 of UMH on the active cluster with the given values file:"
	@echo "  make install VERSION=0.9.12 VALUES_FILE=./test/test-values-full.yaml"
	@echo ""
	@echo "  Upgrade the current cluster to the local version of UMH with the given values file:"
	@echo "  make install CHART=./deployment/united-manufacturing-hub VALUES_FILE=./test/test-values-full.yaml"
	@echo ""
	@echo "  Build and push the docker images for factoryinsight and sensorconnect to the given container registry:"
	@echo "  make docker CTR_REPO=my-repository CTR_IMG='factoryinsight sensorconnect'"


# Print the ports of the services
print-ports:
	@echo "Ports:"
	@echo "Grafana: http://localhost:$(gf_p)"
	@echo "Node-RED: http://localhost:$(node_red_p)/nodered"
	@echo "PostgreSQL: http://localhost:$(db_p)"
	@echo "OPC UA Simulator: http://localhost:$(opc_ua_p)"
	@echo "MQTT Broker: http://localhost:$(mqtt_p)"
	@echo "Kafka: http://localhost:$(kafka_p)"
	@echo "Console: http://localhost:$(console_p)"
