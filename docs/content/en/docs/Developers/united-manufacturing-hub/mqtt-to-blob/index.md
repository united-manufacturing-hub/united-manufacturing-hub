---
title: "mqtt-to-blob"
linkTitle: "mqtt-to-blob"
description: >
  The following guide describes how to catch data from the MQTT-Broker and push them to the MIN.io blob storage
aliases:
  - /docs/Developers/factorycube-server/mqtt-to-blob
  - /docs/developers/factorycube-server/mqtt-to-blob
---
{{% pageinfo color="warning" %}}
**This microservice is still in development and is not considered stable for production use.**
{{% /pageinfo %}}

{{< imgproc mqtttoblob Fit "448x61" >}}{{< /imgproc >}}

MQTT-to-Blob has the function of subscribing to MQTT topics and storing the information into a MIN.io blob storage. The current iteration of MQTT-to-Blob can subscribe to [cameraconnect](/docs/developers/united-manufacturing-hub/cameraconnect)'s MQTT topic and send the image data to MIN.io server's bucket.

The input format of mqtt-to-blob is in accordance with [```/productImage```](/docs/concepts/mqtt/#productimage) JSON format. The information from the JSON is extracted and then stored into an image file in MIN.io with the metadata attached.

Before running the code, the environment variables have to be adjusted for the system. Information regarding the environment variables can be seen in the respective [section](#environment-variables).

## Dependencies

- paho-mqtt==1.6.1
- numpy==1.21.3
- minio==7.1.1

## Environment variables

This chapter explains all used environment variables.

| Variable | Type | Description | Possible values | Example value |
|----|----|----|----|----|
| BROKER_URL | str | Specifies the address to connect to the MQTT-Broker | all DNS names or IP address | 127.0.0.1 |
| BROKER_PORT| int | Specifies the port for the MQTT-Broker | valid port number | 1883 |
| TOPIC | str | MQTT Topic name | Published MQTT Topic name | ia/rawImage/# |
| MINIO_URL | str | Specifies the database DNS name / IP-address for the MIN.io server | all DNS names or IP address with its port number | play.min.io |
| MINIO_SECURE | bool | Select `True` or `False` to activate HTTPS connection or not | `True` or `False` | `True` |
| MINIO_ACCESS_KEY | str | Specifies the username for MIN.io server | eligible username | username |
| MINIO_SECRET_KEY | str | Specifies the password for MIN.io server | password of the user | password |
| BUCKET_NAME | str | Specifies the Bucket name to store the blob in MIN.io server | Bucket name | Bucket-name |
| LOGGING_LEVEL | str | Select level for logging messages | `DEBUG` `INFO` `ERROR` `WARNING` `CRITICAL` | `DEBUG` |

{{< alert title="Note" color="info">}}
- For MINIO_SECURE param, the boolean value has to have capital letter for the first letter
- When using IP address for MINIO_URL, the port number has to be included (e.g. 0.0.0.0:9000)
{{< /alert >}}

## MQTT Connection Return Code
| Return Code | Meaning |
|----|----|
| 0 | Successful Connection |
| 1 | Connection refused - incorrect protocol version |
| 2 | Connection refused - invalid client identifier |
| 3 | Connection refused - server unavailable |
| 4 | Connection refused - Bad username or password |
| 5 | Connection refused - not authorised |
| 6-255 | Currently unused |

## Future work

- In the future, MQTT-to-Blob should also be able to store different kind of data such as sound and video

{{< alert title="Note" color="info">}}
- Use the `dummy_pic_mqtt_topic.json` in the mqtt-to-blob folder manually inject mqtt topic using mqtt explorer
{{< /alert >}}


