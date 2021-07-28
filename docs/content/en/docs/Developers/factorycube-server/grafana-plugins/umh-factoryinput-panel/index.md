---
title: "factoryinput-panel"
linktitle: "factoryinput-panel"
date: 2021-27-07
description: >
# United Manufacturing Hub - Factoryinput Panel
---


## What is United Manufacturing Hub - Factoryinput Panel
UMH Factoryinput Panel allows to easily execute MQTT messages inside the UMH stack from the Grafana Panel.

## Requirements
 - URL of your [grafana-auth](grafana-auth-proxy) proxy server

## Installation

### Build from source

0. Goto ```united-manufacturing-hub/grafana-plugins/umh-factoryinput-panel```


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

- Windows: `C:\Program Files\GrafanaLabs\grafana\data\plugins`
- Linux: `/var/lib/grafana/plugins`

5. Rename the folder to umh-factoryinput-panel
   

6. Enable the [enable development](https://grafana.com/docs/grafana/latest/administration/configuration/) mode to load unsigned plugins


7. Restart your grafana service

### From Grafana's plugin store

TODO

## Usage



## License
 - UMH-Factoryinput Panel: [AGPL](https://raw.githubusercontent.com/Scarjit/united-manufacturing-hub/194-refactor-grafana-datasource/grafana-plugins/umh-factoryinput-panel/LICENSE) 
 - Original Work: [cloudspout-button-panel](https://github.com/cloudspout/cloudspout-button-panel) (MIT)
 - Icons made by [Pixel perfect](https://www.flaticon.com/authors/pixel-perfect) from [www.flaticon.com](https://www.flaticon.com/)
 - `ButtonPayloadEditor` highly influenced from [gapitio/gapit-htmlgraphics-panel](https://github.com/gapitio/gapit-htmlgraphics-panel).
