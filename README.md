<!-- PROJECT LOGO -->
# United Manufacturing Hub

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/united-manufacturing-hub/united-manufacturing-hub)](https://goreportcard.com/report/github.com/united-manufacturing-hub/united-manufacturing-hub)
![Docker Pulls](https://img.shields.io/docker/pulls/unitedmanufacturinghub/factoryinsight)
![Website](https://img.shields.io/website?up_message=online&url=https%3A%2F%2Fwww.united-manufacturing-hub.com)

<img src="docs/static/images/Otto.svg" height="150">

----

## About The Project

The United Manufacturing System is an open source solution for extracting and analyzing data from manufacturing plants and sensors. The Hub includes both software and hardware components to enable the retrofit of productions plants by plug-and-play as well as to integrate existing machine PLCs and IT systems. The result is an end-to-end solution for various questions in manufacturing such as the optimization of production through OEE analysis, preventive maintenance through condition analysis and quality improvement through stop analysis.

- **Open**. open-source (see `LICENSE`) and open and well-documented standard interfaces (MQTT, REST, etc.)
- **Scalable**. Horizontal scaling incl. fault tolerance through Docker / Kubernetes / Helm. Edge devices can be quickly set up and configured in large numbers.
- **Flexible**. Flexible deployment options, from public cloud (Azure, AWS, etc.) to on-premise server installations to Raspberry Pis, everything is possible. Free choice of programming language and systems to be connected through central message broker (MQTT).
- **Tailor-made for production**. Pre-built apps for manufacturing. Use of established automation standards (OPC/UA, Modbus, etc.). Quick connection of production assets either by retrofit or by connecting to existing interfaces.
- **Community and support**. Enterprise support and community for the whole package. Built exclusively on well-documented software components with a large community.
- **Information Security & Data Protection**. Implementation of the central protection goals of information security. High confidentiality through e.g. end-to-end encryption, flexible provisioning options and principle of least privilege. High integrity through e.g. ACID databases and MQTT QoS 2 with TLS. High availability through e.g. use of Kubernetes and (for SaaS) a CDN. 

![Demo](docs/content/en/docs/dashboard.gif)

## Get started

Check out our [Documentation] and [Website] for guides about the United Manufacturing Hub and its architecture, [customer implementations](https://docs.umh.app/docs/examples/) and [articles about Industrial IoT](https://docs.umh.app/docs/concepts/). 

----

## Citations

- Niemeyer, C. & Gehrke, Inga & Müller, Kai & Küsters, Dennis & Gries, Thomas. (2020). Getting Small Medium Enterprises started on Industry 4.0 using retrofitting solutions. Procedia Manufacturing. 45. 208-214. 10.1016/j.promfg.2020.04.096. 
- Kunz, P. (2020). Deep Learning zur industriellen Qualitätsprüfung: Entwicklung eines Plug-and-Play Bildverarbeitungssystems. Institut für Textiltechnik (RWTH Aachen University). 
- Müller, M. (2020). Industrielle Bildverarbeitung zur Qualitätskontrolle in Produktionslinien: Entwicklung einer Entscheidungslogik zur Anwendungsfallspezifischen Auswahl von Hard- und Software. Institut für Textiltechnik (RWTH Aachen University). 
- Altenhofen, N. (2021). Design and Evaluation of a Blue Ocean Strategy for an Open-Core IIoT Platform Provider in the Manufacturing Sector. Institute for Technology and Innovation Management (RWTH Aachen University). 
- Tratner, T. (2021). Implementation of Time Series Data based Digital Time Studies for Manual Processes within the Context of a Learning Factory. Institute of Innovation and Industrial Management (TU Graz).

See also [publications](https://docs.umh.app/docs/publications/) for more material!

<!-- ROADMAP -->
## Roadmap

See the [open issues](https://github.com/united-manufacturing-hub/united-manufacturing-hub/issues) for a list of proposed features (and known issues).

<!-- CONTRIBUTING -->
## Contributing

Contributions are what make the open source community such an amazing place to be learn, inspire, and create. Any contributions you make are **greatly appreciated**. See [`CONTRIBUTING.md`](CONTRIBUTING.md) for more information.

<!-- LICENSE -->
## License

All source code is distributed under the GNU AFFERO GENERAL PUBLIC LICENSE. See [`LICENSE`](LICENSE) for more information. All other components (e.g. trademarks, images, logos, documentation, publications), especially those in the `/docs` folder, are property of the respective owner.

<!-- CONTACT -->
## Contact

[UMH Systems GmbH](https://www.umh.app)

<!-- ACKNOWLEDGEMENTS -->
## Acknowledgements

- [Digital Capability Center Aachen](https://www.mckinsey.com/business-functions/operations/how-we-help-clients/capability-center-network/our-centers/aachen)

<!-- MARKDOWN LINKS & IMAGES -->
<!-- https://www.markdownguide.org/basic-syntax/#reference-style-links -->
[Website]: https://www.umh.app
[Documentation]: https://docs.umh.app/docs/ 
