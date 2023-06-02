<!-- PROJECT LOGO -->
# United Manufacturing Hub

[![License: AGPL v3](https://img.shields.io/badge/License-Apache2.0-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/united-manufacturing-hub/united-manufacturing-hub)](https://goreportcard.com/report/github.com/united-manufacturing-hub/united-manufacturing-hub)
![Docker Pulls](https://img.shields.io/docker/pulls/unitedmanufacturinghub/factoryinsight)
![Website](https://img.shields.io/website?up_message=online&url=https%3A%2F%2Fwww.united-manufacturing-hub.com)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Funited-manufacturing-hub%2Funited-manufacturing-hub.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Funited-manufacturing-hub%2Funited-manufacturing-hub?ref=badge_shield)

<!-- <img src="docs/static/images/Otto.svg" height="150"> -->

The United Manufacturing Hub is an Open-Source [Helm chart for Kubernetes](https://helm.sh/), which combines state-of -the-art IT / OT tools & technologies and brings them into the hands of the engineer.

## What can you do with it?

1. **Extract data from the shopfloor** via [Node-RED](https://learn.umh.app/know/industrial-internet-of-things/tools/#node-red), [sensorconnect](https://umh.docs.umh.app/docs/architecture/microservices/core/sensorconnect/) and [barcodereader](https://https://umh.docs.umh.app/docs/architecture/microservices/community/barcodereader/.umh.app/docs/core/barcodereader/)
2. **Contextualize and standardize data** using [Node-RED](https://learn.umh.app/know/industrial-internet-of-things/tools/#node-red) and our [pre-defined datamodel](https://umh.docs.umh.app/docs/architecture/microservices/community/barcodereader/) (which is compliant with multiple standards)
3. **Exchange and store data** using [MQTT](https://learn.umh.app/know/industrial-internet-of-things/techniques/mqtt/), [Kafka](https://learn.umh.app/know/industrial-internet-of-things/techniques/kafka/) and  TimescaleDB / PostgreSQL,
4. **Visualize data** using [Grafana](https://learn.umh.app/know/industrial-internet-of-things/tools/#grafana) and [factoryinsight](https://umh.docs.umh.app/docs/architecture/microservices/core/factoryinsight/)

## Get started

- [Get started by installing the United Manufacturing Hub](https://learn.umh.app/getstarted/)
- [Visit our learning platform](https://www.umh.app) learn all about common IT, OT and IIoT concepts such as the [**Unified Namespace**](https://learn.umh.app/know/industrial-internet-of-things/techniques/unified-namespace/) and associated common open source tools such as Node-RED, Grafana and the United Manufacturing Hub.

<!-- LICENSE -->
## License

All source code is distributed under the APACHE LICENSE, VERSION 2.0. See [`LICENSE`](LICENSE) for more information. All other components (e.g. trademarks, images, logos, documentation, publications), especially those in the `/docs` folder, are property of the respective owner.

## About our current version 0.x.x.
Our current release, has been deployed and tested at a number of industrial sites worldwide and has proven to be stable in a variety of environments. While we have not yet reached a final version 1.0.0, this is typical for infrastructure projects that require extensive year-long testing and evaluation before a release. We are confident in the stability and reliability of our current release, which has been thoroughly tested over the past few years. As an open-source project, we welcome community involvement and feedback in the ongoing development and refinement of our software. We are committed to ensuring that our software is reliable and ready for deployment in industrial settings.

## Development

To get started with development, please refer to our [development guide](https://umh.docs.umh.app/docs/development/contribute/getting-started/).

You can run `make help` to get a list of all available make targets.

### Dependencies

- [Go](https://golang.org/) version v1.19+.
- [Docker](https://www.docker.com/) version v20.10.11+
- [Helm](https://helm.sh/) version v3.11.2+
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) version v1.26.1+
- [K3d](https://k3d.io/) version v5.4.6+
- GNU Make
- GNU awk
- Python (to run some make targets)

You can download and install all the go dependencies by running:

```bash
make go-deps
```

### Build

To build and push all docker images, run:

```bash
make docker
```

### Run

To run the project on a local k3d cluster, run:

```bash
make cluster-install
```

### Test

To run go unit tests, run:

```bash
make go-test-unit
```

To run upgrade tests, run:

```bash
make kube-test-data-flow-job        # Run the data flow test Job in the current context
make helm-test-upgrade              # Spin up a new cluster, install the latest release and upgrade to the local version
make helm-test-upgrade-with-data    # Spin up a new cluster, install the latest release and upgrade to the local version, then run the data flow test
```

### Utilities

To display the help menu, run:

```bash
make help                     # Print the generic help menu
make <target> PRINT_HELP=y    # Print the help menu for a specific target
```



<!-- CONTACT -->
## Contact

[UMH Systems GmbH](https://www.umh.app)
