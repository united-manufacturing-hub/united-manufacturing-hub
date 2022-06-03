---
title: "kafka-to-blob"
linkTitle: "kafka-to-blob"
description: >
  This microservice saves images from kafka to blob storage.
---

# Getting started

Kafka-to-blob is a microservice that saves images from kafka to blob storage.

This is currently used in combination with cameraconnect.

# Environment Variables

| Variable              | Type    | Description                                              |
|-----------------------|---------|----------------------------------------------------------|
| KAFKA_BOOTSTRAP_SERVER | String  | The kafka bootstrap server.                              |
| KAFKA_LISTEN_TOPIC    | String  | The kafka topic to listen to.                            |
| KAFKA_BASE_TOPIC      | String  | Set this, if you want to automatically create the topic. |
| MINIO_URL             | String  | The url of the minio server.                             |
| MINIO_ACCESS_KEY      | String  | The access key of the minio server.                      |
| MINIO_SECRET_KEY      | String  | The secret key of the minio server.                      |
| BUCKET_NAME           | String  | The bucket to store the images in.                       |
| MINIO_SECURE          | Boolean | Whether to use https or http.                            |

# Guarantees

Kafka-to-blob does not guarantee delivery of messages.