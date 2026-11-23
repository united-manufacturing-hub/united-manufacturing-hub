# Plugins

A plugin adds a feature to an instance, next to its bridges and flows. The **Plugins** tab of the
instance details page lists the plugins added to the instance. The Historian is the first plugin: it
stores the values of the Unified Namespace in TimescaleDB.

| Plugin | What it does | Documentation |
|---|---|---|
| Historian | Saves data contracts from the Unified Namespace to TimescaleDB, to query them with SQL and chart them in Grafana. | [Historian](../historian/README.md) |

## Add a plugin

1. Open the instance and select the **Plugins** tab.
2. Select **Add plugin** and pick the plugin.
3. Fill in the plugin's form and select **Save & Deploy**. For the Historian, see
   [Setup](../historian/setup.md#connection-form).

Each plugin can be added once per instance. A plugin that is already added shows **Already added**.

The **Plugins** tab is disabled when the instance's umh-core version has no plugins. Hovering over it
shows "This instance does not support plugins yet — update umh-core."

## Plugin status

The **Plugins** tab shows one card per added plugin, with its health and its main settings.
**Open plugin** opens the plugin's overview page.

The dot next to the tab name shows the health of the plugin. It stays neutral while no plugin is
added.
