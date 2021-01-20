<!-- PROJECT SHIELDS -->
<!--
*** I'm using markdown "reference style" links for readability.
*** Reference links are enclosed in brackets [ ] instead of parentheses ( ).
*** See the bottom of this document for the declaration of the reference variables
*** for contributors-url, forks-url, etc. This is an optional, concise syntax you may use.
*** https://www.markdownguide.org/basic-syntax/#reference-style-links
-->

<!--
[![Contributors][contributors-shield]][contributors-url]
[![Forks][forks-shield]][forks-url]
[![Stargazers][stars-shield]][stars-url]
[![Issues][issues-shield]][issues-url]
[![MIT License][license-shield]][license-url]
[![LinkedIn][linkedin-shield]][linkedin-url]

-->

<!-- PROJECT LOGO -->
# United Manufacturing Hub

# Contents
- [United Manufacturing Hub](#united-manufacturing-hub)
- [Contents](#contents)
  - [About The Project](#about-the-project)
    - [Architecture & project structure](#architecture--project-structure)
    - [Referenced projects](#referenced-projects)
  - [Getting Started](#getting-started)
  - [Usage](#usage)
  - [Roadmap](#roadmap)
  - [Contributing](#contributing)
  - [License](#license)
  - [Contact](#contact)
  - [Acknowledgements](#acknowledgements)

<!-- ABOUT THE PROJECT -->
## About The Project

[![Product Name Screen Shot][product-screenshot]](https://example.com)

United Manufacturing Hub (UMH) is an open sourcesystem for extracting and analyzing data from manufacturing plants. It is an end-to-end solution for various issues in manufacturing, e.g. optimization of production through OEE analysis (in German: "Betriebsdatenerfassung") and includes software as well as hardware components.

Existing Industry 4.0 platforms are often based on theoretical concepts and research by "big players" in the industry. They tend to develop many elements from scratch and be universal. In contrast, UMH has grown organically and under economic conditions through a wide variety of customer projects in diverse industries. Therefore UMH makes use of a wide range of other open source projects with the focus on solving concrete user problems in production. If you combine all involved projects you can up to xxx millions lines of code and an estimate of xx hundred years of programming time (see also: referenced projects)

### Architecture & project structure

As this projects consists out of various components and sub-components one can easily feel lost. We documented every component, their purpose and how they are integrated:

- for a high level architecture and overview over how data is processed take a look [here](docs/general/dataprocessing.md)
- for the server components take a look [here](docs/server/architecture.md)
- the edge architecture is explained [here](docs/edge/architecture.md)
- An explaination of the overall folder structure can be found [here](docs/folder-structure.md)

### Referenced projects

This projects is built upon millions lines of codes from other open source projects. We want to mention here the most important ones:

- [Grafana] as a dashboarding & visualization tool
- [node-red], to extract and pre-process data on the edge
- [TimescaleDB], to store sequel and time series data
- [Kubernetes] and [Helm], to orchestrate microservices
- [Docker], for containerization

## Getting Started

Check out [Getting Started](docs/getting-started.md) for more information on how to install the server and edge components.

<!-- USAGE EXAMPLES -->
## Usage

Use this space to show useful examples of how a project can be used. Additional screenshots, code examples and demos work well in this space. You may also link to more resources.

_For more examples, please refer to the [Documentation]_

<!-- ROADMAP -->
## Roadmap

- Adding more manufacturing KPIs, like MTBF, MTTR, etc.
- Providing various maintenance options, e.g. charts for Condition Monitoring, Predictive Maintenance or Time-based Maintenance

Additionally, see the [open issues](https://github.com/united-manufacturing-hub/united-manufacturing-hub/issues) for a list of proposed features (and known issues).



<!-- CONTRIBUTING -->
## Contributing

Contributions are what make the open source community such an amazing place to be learn, inspire, and create. Any contributions you make are **greatly appreciated**. See `CONTRIBUTING.md` for more information.


<!-- LICENSE -->
## License

Distributed under the United Public License. The license is basically AGPL with a modification to prevent offering the solution "as-a-service" without contributing back to the community. See `LICENSE` for more information.



<!-- CONTACT -->
## Contact

United Factory Systems GmbH

Project Link: [https://github.com/united-manufacturing-hub/united-manufacturing-hub](https://github.com/united-manufacturing-hub/united-manufacturing-hub)



<!-- ACKNOWLEDGEMENTS -->
## Acknowledgements
* [Digital Capability Center Aachen](https://www.mckinsey.com/business-functions/operations/how-we-help-clients/capability-center-network/our-centers/aachen)

<!-- MARKDOWN LINKS & IMAGES -->
<!-- https://www.markdownguide.org/basic-syntax/#reference-style-links -->
[product-screenshot]: images/screenshot.png
[Documentation]: https://wiki.industrial-analytics.net

<!-- Software -->
[Grafana]: https://github.com/grafana/grafana
[PowerBI]: https://powerbi.microsoft.com/
[node-red]: https://github.com/node-red/node-red
[TimescaleDB]: https://github.com/timescale/timescaledb
[Kubernetes]: https://github.com/kubernetes/kubernetes
[Helm]: https://github.com/helm/helm
[Docker]: https://github.com/docker/engine