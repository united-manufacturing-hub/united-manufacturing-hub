# Values are not updating at regular intervals

In the Topic Browser, the current value of a topic can change at uneven times, the age beside it can grow for longer than you expect, or the History tab can show uneven gaps between values. Most of this is how the Topic Browser sends updates, which [How live updates work](topic-browser.md#how-live-updates-work) explains. The checks below tell that apart from two other causes: a source that publishes less often than you think, and a topic that has stopped updating. Work through them in order.

## Check whether the value still updates

Select the topic and watch the age beside its current value for a minute.

- **The age drops back now and then, at uneven times.** The value updates. Continue with [Check how often the source publishes](#check-how-often-the-source-publishes).
- **The age keeps growing.** No new value has reached the Topic Browser in that time. If the Topic Browser says that the instance is offline, the values return once it is back online. Otherwise work through [Topic does not appear in the Topic Browser](topic-does-not-appear-in-the-topic-browser.md), starting at [Check the Topic Browser of the instance](topic-does-not-appear-in-the-topic-browser.md#check-the-topic-browser-of-the-instance). Its checks also apply to a topic that has stopped updating.

## Check how often the source publishes

The Topic Browser can only show values that reach the Unified Namespace. How often that happens depends on the bridge or stream processor that produces the topic, which the **Bridge** or **Stream** row of the selected topic names. If neither row shows, the **Headers** of the **Details** tab name what produced it.

A source can publish less often than you expect for one of these reasons:

- **Downsampling can suppress unchanged values.** A tag’s age can increase even while the bridge continues reading its device. See [Downsampler](https://docs.umh.app/benthos-umh/processing/downsampler) for details.
- **The bridge reads only on change.** An OPC UA bridge with `subscribeEnabled: true` in its input, for example, gets a value from the server only when the value changes. To read at fixed times instead, set `subscribeEnabled: false` in the OPC UA input of the bridge. The bridge then reads every `pollRate` milliseconds, 1,000 by default, and the downsampler still drops unchanged values.

### Compare the measured and arrival times

Open the **Details** tab and compare **Measured** with **Arrived in UMH**. A large gap between the two has one of two causes: the value was delayed at the device or in the bridge before it reached the Unified Namespace, or the clock of the device differs from the clock of the instance. The Topic Browser does not cause that gap.

## Interpret gaps that the Topic Browser causes

If the source publishes as often as you expect, the gaps you see come from how the Topic Browser sends updates. These are normal:

- **The gaps between values change over time.** The Topic Browser adapts the interval between its batches to the amount of data it has recently read and to the CPU use of its own process. Less data lengthens the gaps, and so does high CPU use.
- **The History tab shows fewer values than the source publishes.** Each batch carries at most one value per topic, the latest. A source that publishes every 100 milliseconds still shows one value per batch, so the History tab is a sample. To keep every reading, save the data contract with a Historian bridge, as [Save data to the Historian](../historian/save-to-historian.md) describes.
- **Values on a quiet instance wait longer.** The Topic Browser decides whether a batch is due only when it reads a new message from the Unified Namespace. On an instance that publishes little, a value can wait until the next message of any topic arrives.

No action is needed for these. The interval of the Topic Browser cannot be configured.

## Interpret values discarded during a backlog

The Topic Browser holds at most 10,000 messages between two batches. When a new message arrives at that limit, it discards the values it holds, keeps the topic tree, and continues collecting. The next batch carries the values that arrived after the discard. This happens while it catches up on a backlog, for example after it was turned off, or on an instance that publishes many messages per second.

- **A topic that publishes continuously** appears again in the next batch, so its History tab can miss some values.
- **A quiet topic** can lose its last value if the discard caught it. The Topic Browser misses that value until the topic publishes again.

If you need every reading, save the data contract with a Historian bridge, as [Save data to the Historian](../historian/save-to-historian.md) describes.

## Getting help

If none of the checks above explains the gaps, contact us through one of the routes on [Get Help](../../get-help.md). Include:

- the instance, the topic, and the times of the gaps,
- what produces the topic and how often it should publish,
- the **Measured** and **Arrived in UMH** times of a recent value,
- the badge and the information icon of the [Topic Browser card](topic-browser.md#the-topic-browser-card),
- the logs of the Topic Browser and of the agent from the time of the gaps, from the **Logs** button on the [Topic Browser card](topic-browser.md#the-topic-browser-card) and on the [Agent card](../instances/agent/README.md#the-agent-card) of the instance.
