<!-- PROJECT LOGO -->
# United Manufacturing Hub

![Connected machines](https://img.shields.io/badge/Connected%20machines-34-informational)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![made-with-Go](https://img.shields.io/badge/Made%20with-Go-1f425f.svg)](http://golang.org)
![Docker Pulls](https://img.shields.io/docker/pulls/unitedmanufacturinghub/factoryinsight)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Funited-manufacturing-hub%2Funited-manufacturing-hub.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Funited-manufacturing-hub%2Funited-manufacturing-hub?ref=badge_shield)
![Website](https://img.shields.io/website?up_message=online&url=https%3A%2F%2Fwww.united-manufacturing-hub.com)

<img src="docs/static/images/Otto.svg" height="150">

----

## About The Project

The United Manufacturing System is an open source solution for extracting and analyzing data from manufacturing plants and sensors. The Hub includes both software and hardware components to enable the retrofit of productions plants by plug-and-play as well as to integrate existing machine PLCs and IT systems. The result is an end-to-end solution for various questions in manufacturing such as the optimization of production through OEE analysis, preventive maintenance through condition analysis and quality improvement through stop analysis.


### Features

#### Open

- open-source (see `LICENSE`)
- Open and well-documented standard interfaces (MQTT, REST, etc.)

#### Scalable

- Horizontal scaling incl. fault tolerance through Docker / Kubernetes / Helm
- Edge devices can be quickly set up and configured in large numbers

#### Flexible

- Flexible deployment options, from public cloud (Azure, AWS, etc.) to on-premise server installations to Raspberry Pis, everything is possible
- Free choice of programming language and systems to be connected through central message broker (MQTT)

#### Tailor-made for production

- Pre-built apps for manufacturing
- Use of established automation standards (OPC/UA, Modbus, etc.)
- Quick connection of production assets either by retrofit or by connecting to existing interfaces

#### Community and support

- Enterprise support and community for the whole package
- Built exclusively on well-documented software components with a large community

#### Information Security & Data Protection

- Implementation of the central protection goals of information security
- High confidentiality through e.g. end-to-end encryption, flexible provisioning options and principle of least privilege
- High integrity through e.g. ACID databases and MQTT QoS 2 with TLS
- High availability through e.g. use of Kubernetes and (for SaaS) a CDN

### Dashboard demo

![Demo](docs/content/en/docs/dashboard.gif)

----

## Getting Started

Check out our [Documentation] for more information on how to install the server and edge components.

### Architecture

![IIoT-stack](docs/content/en/docs/iiot-stack.svg)

<!-- SHOWCASE -->
## Showcase

The United Manufacturing Hub has successfully deployed in various industries, from CNC milling over filling to flame cutting.

Here are some selected cases from our installations across industries, that show how our product is used in practice. For detailed information, please take a look in our [Documentation]

### Flame cutting

<img src="docs/content/en/docs/Examples/flame-cutting.png" height="150">

Plug-and-play retrofit of 11 flame cutting machines and blasting systems at two locations using sensors, barcode scanners and button bars to extract and analyze operating data.


### Brewery

<img src="docs/content/en/docs/Examples/brewery.png" height="150">

Retrofit of a bottling line for different beer types. Focus on the identification of microstops causes and exact delimitation of the bottleneck machine.


### Weaving

<img src="docs/content/en/docs/Examples/weaving.png" height="150">

Retrofit of several weaving machines that do not provide data via the PLC to extract operating data to determine the OEE and detailed breakdown analysis of the individual key figures.


<!-- ROADMAP -->
## Roadmap

See the [open issues](https://github.com/united-manufacturing-hub/united-manufacturing-hub/issues) for a list of proposed features (and known issues).

<!-- CONTRIBUTING -->
## Contributing

Contributions are what make the open source community such an amazing place to be learn, inspire, and create. Any contributions you make are **greatly appreciated**. See [`CONTRIBUTING.md`](CONTRIBUTING.md) for more information.

<!-- LICENSE -->
## License

All source code is distributed under the GNU AFFERO GENERAL PUBLIC LICENSE. See [`LICENSE`](LICENSE) for more information. All other components (e.g. trademarks, images, logos) are property of the respective owner.

<!-- CONTACT -->
## Contact

[United Factory Systems GmbH](https://www.united-manufacturing-hub.com)

<!-- ACKNOWLEDGEMENTS -->
## Acknowledgements

- [Digital Capability Center Aachen](https://www.mckinsey.com/business-functions/operations/how-we-help-clients/capability-center-network/our-centers/aachen)

<!-- MARKDOWN LINKS & IMAGES -->
<!-- https://www.markdownguide.org/basic-syntax/#reference-style-links -->
[Website]: https://www.united-manufacturing-hub.com
[Documentation]: https://docs.umh.app 
