---
title: "Kafka"
linkTitle: "Kafka"
weight: 2
description: >
  This documents our Kafka structure and settings
---

### Default settings
By default, the following important settings are used:

| setting           | value     | description                                           |
|-------------------|-----------|-------------------------------------------------------|
| `retention.ms`    | 604800000 | After 7 days messages will be deleted                 |
| `retention.bytes` | -1        | We don't limit the amount of messages stored by Kafka |
