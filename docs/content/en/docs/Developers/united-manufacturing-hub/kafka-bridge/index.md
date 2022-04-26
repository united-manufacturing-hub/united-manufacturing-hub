---
title: "kafka-bridge"
linkTitle: "kafka-bridge"
description: >
  This microservice is a bridge between two Kafka brokers.
---

# Getting started

Kafka-bridge is a microservice that connects two Kafka brokers,
and forwards messages between them.

It is used, when having one or multiple edge devices and a central server.

# Environment Variables

The following environment variables are used by the microservice:

| Variable                      | Type   | Description                                                                         |
|-------------------------------|--------|-------------------------------------------------------------------------------------|
| LOGGING_LEVEL                 | String | Configures the zap logging level, set to DEVELOPMENT to enable development logging. |
| KAFKA_TOPIC_MAP               | JSON   | Configures which topics should be forwarded.                                        |
| LOCAL_KAFKA_BOOTSTRAP_SERVER  | String | Configures the local Kafka bootstrap server.                                        |
| REMOTE_KAFKA_BOOTSTRAP_SERVER | String | Configures the remote Kafka bootstrap server.                                       |

## Kafka Topic Map

The ``KAFKA_TOPIC_MAP`` has the following JSON schema:
````json
{
    "$schema": "http://json-schema.org/draft-07/schema",
    "type": "array",
    "title": "Kafka Topic Map",
    "description": "This schema validates valid Kafka topic maps.",
    "default": [],
    "examples": [
      [
        {
          "name":"HighIntegrity",
          "topic":"^ia\\..+\\..+\\..+\\.(?!processValue).+$",
          "bidirectional":false,
          "send_direction":"to_remote"
        }
      ],
      [
        {
          "name":"HighIntegrity",
          "topic":"^ia\\..+\\..+\\..+\\.(?!processValue).+$",
          "bidirectional":true
        },
        {
          "name":"HighThroughput",
          "topic":"^ia\\..+\\..+\\..+\\.(processValue).*$",
          "bidirectional":false,
          "send_direction":"to_remote"
        }
      ]
    ],
    "additionalItems": true,
    "items": {
        "$id": "#/items",
        "anyOf": [
            {
                "$id": "#/items/anyOf/0",
                "type": "object",
                "title": "Unidirectional Kafka Topic Map with send direction",
                "description": "This schema validates entries, that are unidirectional and have a send direction.",
                "default": {},
                "examples": [
                    {
                        "name": "HighIntegrity",
                        "topic": "^ia\\..+\\..+\\..+\\.(?!processValue).+$",
                        "bidirectional": false,
                        "send_direction": "to_remote"
                    }
                ],
                "required": [
                    "name",
                    "topic",
                    "bidirectional",
                    "send_direction"
                ],
                "properties": {
                    "name": {
                        "$id": "#/items/anyOf/0/properties/name",
                        "type": "string",
                        "title": "Entry Name",
                        "description": "Name of the map entry, only used for logging & tracing.",
                        "default": "",
                        "examples": [
                            "HighIntegrity"
                        ]
                    },
                    "topic": {
                        "$id": "#/items/anyOf/0/properties/topic",
                        "type": "string",
                        "title": "The topic to listen on",
                        "description": "The topic to listen on, this can be a regular expression.",
                        "default": "",
                        "examples": [
                            "^ia\\..+\\..+\\..+\\.(?!processValue).+$"
                        ]
                    },
                    "bidirectional": {
                        "$id": "#/items/anyOf/0/properties/bidirectional",
                        "type": "boolean",
                        "title": "Is the transfer bidirectional?",
                        "description": "When set to true, the bridge will consume and produce from both brokers",
                        "default": false,
                        "examples": [
                            false
                        ]
                    },
                    "send_direction": {
                        "$id": "#/items/anyOf/0/properties/send_direction",
                        "type": "string",
                        "title": "Send direction",
                        "description": "Can be either 'to_remote' or 'to_local'",
                        "default": "",
                        "examples": [
                            "to_remote",
                            "to_local"
                        ]
                    }
                },
                "additionalProperties": true
            },
            {
                "$id": "#/items/anyOf/1",
                "type": "object",
                "title": "Bi-directional Kafka Topic Map with send direction",
                "description": "This schema validates entries, that are bi-directional.",
                "default": {},
                "examples": [
                    {
                        "name": "HighIntegrity",
                        "topic": "^ia\\..+\\..+\\..+\\.(?!processValue).+$",
                        "bidirectional": true
                    }
                ],
                "required": [
                    "name",
                    "topic",
                    "bidirectional"
                ],
                "properties": {
                    "name": {
                        "$id": "#/items/anyOf/1/properties/name",
                        "type": "string",
                        "title": "Entry Name",
                        "description": "Name of the map entry, only used for logging & tracing.",
                        "default": "",
                        "examples": [
                            "HighIntegrity"
                        ]
                    },
                    "topic": {
                        "$id": "#/items/anyOf/1/properties/topic",
                        "type": "string",
                        "title": "The topic to listen on",
                        "description": "The topic to listen on, this can be a regular expression.",
                        "default": "",
                        "examples": [
                            "^ia\\..+\\..+\\..+\\.(?!processValue).+$"
                        ]
                    },
                    "bidirectional": {
                        "$id": "#/items/anyOf/1/properties/bidirectional",
                        "type": "boolean",
                        "title": "Is the transfer bidirectional?",
                        "description": "When set to true, the bridge will consume and produce from both brokers",
                        "default": false,
                        "examples": [
                            true
                        ]
                    }
                },
                "additionalProperties": true
            }
        ]
    }
}
````

### Example

````json
[
   {
      "name":"HighIntegrity",
      "topic":"^ia\\..+\\..+\\..+\\.(?!processValue).+$",
      "bidirectional":true
   },
   {
      "name":"HighThroughput",
      "topic":"^ia\\..+\\..+\\..+\\.(processValue).*$",
      "bidirectional":false,
      "send_direction":"to_remote"
   }
]
````

This map would sync every non processValue topic from both brokers.

It will also send the processValue messages to the remote broker.

# Kubernetes usage

Inside kubernetes values.yaml you can use a normal YAML map to do the configuration.

```yaml
  kafkaBridge:
    enabled: true
    remotebootstrapServer: ""
    topicmap:
      - name: HighIntegrity
        topic: '^ia\..+\..+\..+\.(addMaintenanceActivity)|(addOrder)|(addParentToChild)|(addProduct)|(addShift)|(count)|(deleteShiftByAssetIdAndBeginTimestamp)|(deleteShiftById)|(endOrder)|(modifyProducedPieces)|(modifyState)|(productTag)|(productTagString)|(recommendation)|(scrapCount)|(startOrder)|(state)|(uniqueProduct)|(scrapUniqueProduct).+$'
        bidirectional: true
      - name: HighThroughput
        topic: '^ia\\..+\\..+\\..+\\.(processValue).*$'
        bidirectional: false
        send_direction: to_remote
```

# Guarantees

This microservice provides at-least-once guarantees, by manually committing the offset of the message that was processed.
