# Edit the Config File

This page explains how to edit an instance's `config.yaml` in the Management Console, and how to repair the file when umh-core cannot read it. The repair steps need access to the Docker host.

`config.yaml` holds everything you configure on an instance: bridges, stand-alone flows, data models and the agent settings. The UI writes to the same file, so a bridge you create in the UI appears in `config.yaml`, and a bridge you add to `config.yaml` appears in the UI. The [Configuration Reference](../../reference/configuration-reference.md) lists every key.

## Open the file

Open the instance's details page and click **View Instance Config**. In the instance list, the row menu of an online UMH Core instance has the same page under **Config file**.

The page shows `/data/config.yaml` as umh-core read it last. **Refresh** reads it again. **Download** saves a copy named `config-<first 8 characters of the instance ID>.yaml`.

## Save a change

1. Edit the file in the editor.
2. Click **Save**.

The Management Console first checks that the text is valid YAML. umh-core then checks that the file matches the structure of `config.yaml` and writes it. It rejects the save if the file changed on the instance since you opened it, for example because someone created a bridge in the meantime. Click **Refresh** and make your change again.

After **Configuration saved successfully**, umh-core applies the change without a restart.

{% hint style="warning" %}
The check on save accepts a key that umh-core does not know, for example a misspelled one. umh-core reads `config.yaml` strictly afterwards, so a single unknown key makes it stop applying changes. See [When umh-core cannot read the file](#when-umh-core-cannot-read-the-file).
{% endhint %}

## When umh-core cannot read the file

While umh-core cannot read `config.yaml`, it applies no changes. Bridges, flows and Redpanda keep running with the configuration they had. The Config File page cannot load the file either and shows an error.

After 10 minutes of failed reads, umh-core stops its control loop. It then no longer starts, stops or restarts any component until the container restarts. The [Agent metrics](agent/README.md#metrics) count this as an error with `instance="config_permanent_failure"`.

To repair the file, run on the Docker host:

```bash
docker exec -it <container name> vi /data/config.yaml
```

The agent log at `/data/logs/umh-core/current` names the line that umh-core could not read. Fix it and save. If umh-core already stopped its control loop, restart the container afterwards:

```bash
docker restart <container name>
```

`docker restart` keeps the container's environment variables and its `/data` volume.
