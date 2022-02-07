---
title: "How to add new MQTT messages to mqtt-to-postgresql"
linktitle: "How to add new MQTT messages to mqtt-to-postgresql"
description: >
  For new developers the internal structure of mqtt-to-postgresql might not be self-explaining. Therefore, this tutorial.
---

In general MQTT messages in [mqtt-to-postgresql](../united-manufacturing-hub/mqtt-to-postgresql/) are first received (see [entrypoint](#entrypoint)) and then stored in a message specific queue. Per message type: All messages within the last second are then gathered together and written into the database.

If one of the messages fails to get written into the database the entire batch of messages is regarded. In the future we should add here an additional buffer that tries each message separately to first identify the broken messages from the rest of the batch and then repeatedly tries to add the broken messages for like 5 times over a time period of 30 minutes to prevent issues with delayed messages (e.g., creation of a product is stuck somewhere on the edge, but the product itself is already used in a following station).

## Entrypoint

The entrypoint is in `mqtt.go`:

https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/5177804744f0ff0294b822e85b4967655c2bc9fb/golang/cmd/mqtt-to-postgresql/mqtt.go#L83-L112

From there you can work and copy paste yourself until you add the messages into the queue. 

Tip: for parsing the MQTT JSON messages we should use quicktype.io with the MQTT JSON message example from the documentation.

https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/5177804744f0ff0294b822e85b4967655c2bc9fb/golang/cmd/mqtt-to-postgresql/dataprocessing.go#L84

## Queue

The next function is in `database.go`:

https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/5177804744f0ff0294b822e85b4967655c2bc9fb/golang/cmd/mqtt-to-postgresql/database.go#L1050-L1077

## Testing

For testing we recommend spinning up a local instance of the entire `united-manufacturing-hub` Helm chart and then sending messages with Node-RED.
