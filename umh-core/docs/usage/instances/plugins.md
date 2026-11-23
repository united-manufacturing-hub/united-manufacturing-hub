# Plugins

A plugin adds a function to an instance that UMH Core does not have on its own. You add a plugin to one instance at a time. The Historian is the first plugin.

## The Plugins tab

The **Plugins** tab of the instance details page shows one card per plugin added to the instance, and an **Add a plugin** card.

A plugin card shows the plugin's health badge and a summary of its state. **Open plugin** opens the plugin's own pages. The menu next to it lists further pages of the plugin and **Delete**, which removes the plugin from the instance.

**Add plugin** on the **Add a plugin** card opens **Select a plugin**, which lists the plugins you can add. A plugin that the instance already has shows **Already added**.

The tab is unavailable when plugins are not enabled for your organization, or when the instance's UMH Core version does not support them. In the second case, hovering over the tab shows `This instance does not support plugins yet — update umh-core.`

## Available plugins

| Plugin | What it does | Page |
|---|---|---|
| Historian | Saves UNS data contracts to TimescaleDB, where you query them in Grafana. | [Historian](../historian/README.md) |
