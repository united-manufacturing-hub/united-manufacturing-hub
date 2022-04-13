---
title: "Kafka"
linkTitle: "Kafka"
weight: 2
description: >
  This documents our Kafka structure and settings
---

# Default settings
By default, the following important settings are used:

| setting           | value     | description                                           |
|-------------------|-----------|-------------------------------------------------------|
| `retention.ms`    | 604800000 | After 7 days messages will be deleted                 |
| `retention.bytes` | -1        | We don't limit the amount of messages stored by Kafka |

# Topics

Our Kafka topics are structured as follows:

```
  ia.CUSTOMER.LOCATION.MACHINE.EVENT
```

There are two exception to this rule:
- ```ia.raw.TRANSMITTERID.GATEWAYSERIALNUMBER.PORTNUMBER.IOLINKSENSORID```
- ```ia.rawImage.TRANSMITTERID.CAMERAMACADDRESS```

Specifications for those can be found on the [UMH datamodel](https://docs.umh.app/docs/concepts/mqtt/) page. 


All names have to match the following regex:
```regexp
^[a-zA-Z0-9_\-]$
```

## Customer

This is name of the customer (e.g: united-manufacturing-hub).
It can also be an abbreviation (e.g: umh) of the customer name.

## Location

This is the name of the location, the sender belongs to.
It can be a physical location (aachen), an virtual location (rack-1), or any other unique specifier.

## Machine

This is the name of the machine, the sender belongs to.
It can be a physical machine (printer-1), a virtual machine (vmachine-1), or any other unique specifier.

## Event

Our kafka stack currently supports the following events:

 - addMaintenanceActivity
 - addOrder
 - addParentToChild
 - addProduct
 - addShift
 - count
 - deleteShiftByAssetIdAndBeginTimestamp
 - deleteShiftById
 - endOrder
 - modifyProducedPieces
 - modifyState
 - processValue
 - processValueFloat64
 - processValueString
 - productTag
 - productTagString
 - recommendation
 - scrapCount
 - startOrder
 - state
 - uniqueProduct
 - scrapUniqueProduct

Further information about these events can be found at the [UMH datamodel](https://docs.umh.app/docs/concepts/mqtt/) site.


# Routing

Below you can find an example flow of messages.

![Example kafka flow](flow.drawio.svg)

## Edge PC
In this example, we have an edge pc, which is connected to multiple sensors and a camera.
It also receives data via MQTT, Node-RED and a barcode reader.

In our dataflow, we handle any IO-Link compatible sensor with sensorconnect, which reads IO-Link data and publishes it to Kafka.
Compatible cameras / barcode readers are handled by cameraconnect and barcode reader respectively.

Node-RED can be used to pre-process arbitrary data and the publish it to Kafka.

MQTT-Kafka-Bridge takes MQTT data and publishes it to Kafka.

Once the data is published to the Edge Kafka broker, other microservices can subscribe to the data and produce higher level data,
which gets re-published to the Edge Kafka broker.

## Kafka bridge

This microservice can sit on either the edge or server and connects two kafka brokers.
It has a regex based filter, for sending messages, which can be bi- or uni-directional.
Also, it filters out duplicated messages, to prevent loops.

Every bridge adds an entry to the kafka header, to identify the source of the message and all hops taken.

## Server

On the server, we have two microservices listening for incoming data.

Kafka-to-blob is a microservice which listens to Kafka and publishes the data to blob storage, in our case Minio.
Kafka-to-postgresql is a microservice which listens to Kafka and publishes the data to a PostgreSQL database,
with timescale installed.

# Guarantees

Our system is build to provide at-least-once delivery guarantees, once a message first enters any kafka broker,
except for high throughput data (processValue, processValueFloat64, processValueString).

Every message taken out of the broker, will only get committed to the broker, once it has been processed, or
successfully returned to the broker (in case of an error).

For this, we use the following kafka settings:

```json
{
  "enable.auto.commit":       true,
  "enable.auto.offset.store": false,
  "auto.offset.reset":        "earliest"
}
```

- enable.auto.commit 
  - This will auto commit all offsets, in the local offset store, every couple of seconds.
- enable.auto.offset.store 
  - We manually store the offsets in the local offset store, once we confirmed, that the message has been processed.
- auto.offset.reset
  - This will return the offset pointer to the earliest unprocessed message, in case of a re-connect.

Note, that we could have gone for disabling "enable.auto.commit", but in our testing, that was significantly slower.

For in-depth information about how we handle message inside our microservices, please see their documentation:

- [Kafka-to-blob](https://docs.umh.app/docs/developers/united-manufacturing-hub/kafka-to-blob/)
- [Kafka-to-postgresql](https://docs.umh.app/docs/developers/united-manufacturing-hub/kafka-to-postgresql/)
- [Kafka-bridge](https://docs.umh.app/docs/developers/united-manufacturing-hub/kafka-bridge/)