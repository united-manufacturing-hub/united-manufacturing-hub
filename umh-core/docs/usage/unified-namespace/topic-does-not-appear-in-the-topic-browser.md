# Topic does not appear in the Topic Browser

The Topic Browser can show an empty tree — `No topics found. Ensure your UMH instance is connected and generating topics.` — while the instance is connected and producing data. A single topic can also stay missing while its bridge keeps sending it. This page explains what to try and how to check that your data is still flowing. The last step needs access to the Docker host of the instance.

An empty tree does not mean your data stopped. The Topic Browser is a view the instance builds from the messages it receives (see [Topic Browser](topic-browser.md)); the messages themselves keep flowing through your bridges into [Redpanda](redpanda.md). The steps below recover the view when it stops showing what the instance has.

## What to try first

1. Wait a minute. A new topic can take a moment to show.
2. Reload the page in your browser.
3. Restart the container of the instance.

   {% hint style="warning" %}
   Restarting the container stops umh-core and every bridge in it until it is back up, and the bridges read no values during the restart. Plan it for a maintenance window.
   {% endhint %}

## Check whether the data is still flowing

Open the bridge that sends the topic under [Bridges](../data-flows/bridges.md) and select **Metrics**. While data flows, the messages received and sent keep counting up.

The [Redpanda](redpanda.md) card on the instance's detail page shows the state of the broker the messages go to.

If the bridge shows errors or its sent count stands still, the topic is missing upstream of the Topic Browser: see [Bridges Do Not Start](../data-flows/bridges-do-not-start.md) and the [Bridges](../data-flows/bridges.md) page. To follow a single message from its device tag to its topic, see [Metadata and Tracing](metadata-and-tracing.md).

## Still missing

If none of the steps above helps, send us what you see — the instance, the time, and whether a reload or a restart brought the topic back — on [Discord](https://discord.gg/F9mqkZnm8U) or through your usual UMH contact.
