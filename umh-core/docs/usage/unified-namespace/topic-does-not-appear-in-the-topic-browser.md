# Topic does not appear in the Topic Browser

The Topic Browser can show `No topics have arrived yet.`, or miss a single topic, while your bridges and stream processors publish data. Often the data has not reached the Topic Browser yet. The checks below show whether that is the case or whether something else is wrong.

![The Topic Browser with an empty topic tree](../../generated/topic-browser-empty-tree.png)

## What to try first

Three checks, in order of how little they disturb:

1. Wait a minute. A new topic can take a moment to show.
2. Reload the page in your browser.
3. Open the [Topic Browser card](topic-browser.md#the-topic-browser-card) on the instance's detail page. If its badge does not read **Active**, the information icon beside the badge gives the reason.

## When the instance is offline

If the instance that reports the topic has gone offline, the browser says so above the value and keeps showing what arrived before it went. The age beside the value says how long ago that was.

![A topic whose instance went offline, with the last values it reported](../../generated/topic-browser-instance-offline.png)

## When the Topic Browser is turned off

If `internal.topicbrowser.desiredState` is `stopped` in the `config.yaml` of the instance, the Topic Browser collects no topics on that instance and the tree stops updating. An empty tree or a missing topic is then expected. See [Configuration](topic-browser.md#configuration).

## Check whether the data is still flowing

If the topic shows in the tree, select it and read the **Bridge** or **Stream** row to find out what produces it. If it does not show, find the bridge or stream processor that should produce it under **Data Flows**.

- For a bridge, open it and select **Metrics**, as [Bridges](../data-flows/bridges.md) describes.
- For a stream processor, open it and select **Metrics** on its Monitoring card, as [Stream Processors](../data-flows/stream-processor.md#metrics) describes.

While data flows, the counts of messages received and sent keep rising.

If the count of messages sent stands still, the topic is missing upstream of the Topic Browser. See [Bridges Do Not Start](../data-flows/bridges-do-not-start.md) for a bridge that never starts, the [Bridges](../data-flows/bridges.md) page for one that starts and sends nothing, and [Stream Processors](../data-flows/stream-processor.md) for a stream processor.

If the count keeps rising but errors show, they may concern other topics: one bridge or stream processor can send many. Open its **Logs** and check whether the errors concern the data behind your topic. If they do, fix what they report and check the counts again.

To follow a single message from its device tag to its topic, see [Metadata and Tracing](metadata-and-tracing.md).

The [Redpanda](redpanda.md) card on the instance's detail page shows the state of the broker the messages go to. If it does not read **Active**, see [When Redpanda is degraded](redpanda.md#when-redpanda-is-degraded).

## Restart the container

This step requires access to the instance’s Docker host.

{% hint style="warning" %}
Restarting the container stops umh-core and every data flow in it. Nothing is read from your devices until it is back up, which takes about a minute. Plan it for a maintenance window.
{% endhint %}

If the data is flowing and the topic is still missing, restart the container of the instance. On the Docker host, run:

```bash
docker restart umh-core
```

Replace `umh-core` with the name of your container if you named it differently.

Topics that send data infrequently may take longer to reappear in the Topic Browser after a UMH Core restart.

## Getting help

If none of the steps above helps, tell us on [Discord](https://discord.gg/F9mqkZnm8U) or through your usual UMH contact: the instance, the time, and whether a reload or a restart brought the topic back.
