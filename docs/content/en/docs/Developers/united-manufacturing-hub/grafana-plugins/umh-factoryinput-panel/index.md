---
title: "factoryinput-panel"
linktitle: "factoryinput-panel"
date: 2021-27-07
description: Documentation of factoryinput-panel
aliases:
  - /docs/Developers/factorycube-server/grafana-plugins/umh-factoryinput-panel
  - /docs/developers/factorycube-server/grafana-plugins/umh-factoryinput-panel
---

**This microservice is still in development and is not considered stable for production use.**

## Getting started
UMH Factoryinput Panel allows to easily execute MQTT messages inside the UMH stack from the Grafana Panel.

## Requirements
 - A united manufacturing hub stack
 - External IP or URL of the [grafana-proxy](/docs/developers/united-manufacturing-hub/grafana-proxy) server.
   - In most cases it is the same IP as your Grafana dashboard

## Installation
If you have installed the UMH-Stack as described in our quick start Tutorial, then this plugin is already installed on your Grafana installation

If you want to develop this Panel further, please follow the instructions below

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


## Usage
### Prerequisites
1. Open your Grafana instance
2. Log in
3. Open your Profile and check if your organization name inside Grafana matches the rest of your UMH stack

### Creating a new Panel
1. Create a new Dashboard or edit an existing one
2. Click "Add an empty panel"
3. On the right sidebar switch the Visualization to "Button Panel"
4. Fill out the fields inside "REST Integration"
   1. URL
      - ```http://{URL to your grafana-proxy}/api/v1/factoryinput/```
      - Example:
        - ```http://172.21.9.195:2096/api/v1/factoryinput/```
   2. Location
      - Location of your Asset
   3. Asset
      - Name of the Asset
   4. Value
      - MQTT prefix
        - Example prefixes:
          - count
          - addShift
          - modifyShift
   5. Customer
      - Your organization name
   6. Payload
      - JSON encoded payload to send as MQTT message payload
5. Modify any additional options are you like
6. When you are finished customizing, click on "Apply"

## Example Panel
{{< imgproc grafana_panel.png Fit "500x300" >}}{{< /imgproc >}}

## Notes
1. Clicking the button will immediately send the MQTT message, through our HTTP->MQTT stack. Please don't send queries modifying date you would later need !

## Common pitfalls
 - Pressing the button just changes the cog to an warning sign 
   1. Open your network inspector view (Ctrl+Shift+I on Chrome)
   2. Press the button again
   3. If no request appears, then you haven't filled out all required fields
   4. Your request shows:
      - 403
         - Make sure the customer field is set to your grafana organization name
      - 400
        - Your request was incorrectly formatted
        - Check that the URL is in the format specified above
        - Check if your payload contains valid JSON
          - You can validate your payload [here](https://jsonformatter.curiousconcept.com/)
        - Check that the Value field matches a valid MQTT command
   

## Technical information
Below you will find a schematic of this flow, through our stack

{{< imgproc grafana_to_mqtt_stack.png Fit "1920x1080" >}}{{< /imgproc >}}


## License
 - Original Work: [cloudspout-button-panel](https://github.com/cloudspout/cloudspout-button-panel) (MIT)
 - Icons made by [Pixel perfect](https://www.flaticon.com/authors/pixel-perfect) from [www.flaticon.com](https://www.flaticon.com/)
 - `ButtonPayloadEditor` highly influenced from [gapitio/gapit-htmlgraphics-panel](https://github.com/gapitio/gapit-htmlgraphics-panel).
