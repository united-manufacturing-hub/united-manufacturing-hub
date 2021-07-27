---
title: "umh-datasource"
linktitle: "umh-datasource"
date: 2021-27-07
description: >
# United Manufacturing Hub - Datasource
---

## What is United Manufacturing Hub Datasource?
UMH Datasource provides an Grafana 8.X compatible plugin, allowing easy data extraction from the UMH factoryinsight microservice.


## Installation
### Build from source

0. Clone the datasource repo ```git@github.com:united-manufacturing-hub/united-manufacturing-hub-datasource.git```


1. Install dependencies
```BASH
yarn install
```
2. Build plugin in development mode or run in watch mode
```BASH
yarn dev
```
3. Build plugin in production mode (not recommended due to [Issue 32336](https://github.com/grafana/grafana/issues/32336))
```BASH
yarn build
```
4. Move the resulting dist folder into your grafana plugins directory
- Windows: ```C:\Program Files\GrafanaLabs\grafana\data\plugins```
- Linux: ```/var/lib/grafana/plugins```
5. Rename the folder to umh-datasource


6. You need to [enable development](https://grafana.com/docs/grafana/latest/administration/configuration/) mode to load unsigned plugins


7. Restart your grafana service

### From Grafana's plugin store
TODO
