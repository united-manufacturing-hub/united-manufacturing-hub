---
title: "The UMH datamodel / MQTT"
linkTitle: "The UMH datamodel / MQTT"
weight: 2
description: >
  All events or subsequent changes in production are transmitted via MQTT in the following data model
---

## Introduction

All events or subsequent changes in production are transmitted via MQTT in the following data model. This ensures that all participants are always informed about the latest status.

The data model in the MQTT Broker can be divided into four levels. In general, the higher the level, the lower the data frequency and the more the data is prepared.

If you do not know the idea of MQTT (important keywords: "broker", "subscribe", "publish", "topic"), we recommend reading the [wikipedia article](https://en.wikipedia.org/wiki/MQTT) first.

All MQTT messages consist out of one JSON with at least two elements in it:

| Key | Data type/format | Description |
|----|----|----|
|`timestamp_ms`| int | the amount of milliseconds since the 1970-01-01 (also called [UNIX timestamp](https://en.wikipedia.org/wiki/Unix_time) in milliseconds)|
|`<valueName>`| int, str, dict | a value that can be int, str, or even in dict format |

{{% alert title="Example" color="primary" %}}
{{< tabpane langEqualsHeader=false >}}

{{< tab header="str" lang=JSON >}}
{
    "timestamp_ms": 1588879689394,
    "valueName": "str"
}
{{< /tab >}}

{{< tab header="int" lang=JSON >}}
{
    "timestamp_ms": 1588879689394,
    "valueName": 24
}
{{< /tab >}}

{{< tab header="dict" lang=JSON >}}
{
    "timestamp_ms": 1588879689394,
    "valueName": {
        "key1": "some text",
        "key2": 12
    }
}
{{< /tab >}}

{{< /tabpane >}}
{{% /alert %}}

{{< alert title="Note" color="info">}}
- Some messages might deviate from this format. When this happens it will be noted explicitly. 
- All topics are to be written in lower case only!
{{< /alert >}}


## 1st level: Raw data

Data from this level are all raw data, which are not yet contextualized(i.e., assigned to a machine). These are, in particular, all data from [sensorconnect] and [cameraconnect].

### Topic: ia/raw/

Topic structure: `ia/raw/<transmitterID>/<gatewaySerialNumber>/<portNumber>/<IOLinkSensorID>`

All raw data coming in via [sensorconnect].

{{% alert title="Example for ia/raw/" color="primary" %}}
Topic: `ia/raw/2020-0102/0000005898845/X01/210-156`

This means that the transmitter with the serial number `2020-0102` has one ifm gateway connected to it with the serial number `0000005898845`. This gateway has the sensor `210-156` connected to the first port `X01`.

```JSON
{
  "timestamp_ms": ..., // int
  "type": "IO-Link", // in case of IO-Link
  "connected": 1, // 1 if connected
  "<value1Name>": <convertedValue1>, //for example: "distance": 16,
  "<value2Name>": <convertedValue2>,
  ...
  "<valueNName>": <convertedValueN>
}
```

If the device is not known from one of the IODD-XML files, the received valueString is directly written into the JSON message:
```JSON
{
  "timestamp_ms": ...,
  "type": "IO-Link", //in case of IO-Link
  "connected": 1, // 1 in case of connected
  "value_string": <unconvertedValue>
}
```

If the connection can not be established:
```JSON
{
  "timestamp_ms": ...,
  "type": "IO-Link", //in case of IO-Link
  "connected": 0, // 0 in case of not connected, also no value included in JSON
}
```

{{% /alert %}}

### Topic: ia/rawImage/

Topic structure: `ia/rawImage/<TransmitterID>/<MAC Adress of Camera>`

All raw data coming in via [cameraconnect].


| key | data type | description |
|-----|-----|--------------------------|
|`image_id` | str | a unique identifier for every image acquired (e.g. format:`<MACaddress>_<timestamp_ms>`) |
|`image_bytes` | str | base64 encoded image in JPG format in bytes |
|`image_height` | int | height of the image in pixel |
|`image_width` | int | width of the image in pixel |
|`image_channels` | int | amount of included color channels (Mono: 1, RGB: 3) |

{{% alert title="Example for ia/rawImage/" color="primary" %}}
Topic: `ia/rawImage/2020-0102/4646548`

This means that the transmitter with the serial number 2020-0102 has one camera connected to it with the serial number 4646548.

```json
{
	"timestamp_ms": 214423040823,
	"image":  {
		"image_id": "<MACaddress>_<timestamp_ms>",
		"image_bytes": "3495ask484...",
		"image_height": 800,
		"image_width": 1203,
		"image_channels": 3
	}
}
```
{{% /alert %}}

{{% alert title="Example for decoding an image and saving it locally with OpenCV" color="primary" %}}
```python
im_bytes = base64.b64decode(incoming_mqtt_message["image"]["image_bytes"])
im_arr = np.frombuffer(im_bytes, dtype=np.uint8)  # im_arr is a one-dimensional Numpy array
img = cv2.imdecode(im_arr, flags=cv2.IMREAD_COLOR)
cv2.imwrite(image_path, img)
```
{{% /alert %}}
  
## 2nd level: Contextualized data

In this level the data is already assigned to a machine.

Topic structure: `ia/<customerID>/<location>/<AssetID>/<Measurement>` e.g. `ia/dccaachen/aachen/demonstrator/count`.

An asset can be a step, machine, plant or line. It uniquely identifies the smallest location necessary for modeling the 
process.

By definition all topic names should be lower case only!

### /count

Topic: `ia/<customerID>/<location>/<AssetID>/count`

Here a message is sent every time something has been counted. This can be, for example, a good product or scrap.

`count` in the JSON is an integer.
`scrap` in the JSON is an integer, which is optional. It means `scrap` pieces of `count` are scrap. If not specified it is 0 (all produced goods are good).

| key | data type | description |
|-----|-----|--------------------------|
| `count` | int | quantity of produced item|

{{% alert title="Example for /count" color="primary" %}}
```json
{
    "timestamp_ms": 1588879689394, 
    "count": 1
}
```

{{% /alert %}}
### /scrapCount

Topic: `ia/<customerID>/<location>/<AssetID>/scrapCount`

Here a message is sent every time products should be marked as scrap. It works as follows:
A message with `scrap` and `timestamp_ms` is sent. It starts with the count that is directly before `timestamp_ms`. It is now iterated step by step back in time and step by step the existing counts are set to scrap until a total of `scrap` products have been scraped.

{{< imgproc scrapCount Fit "500x300" >}}{{< /imgproc >}}

{{% alert title="Important notes" color="warning"%}}

- You can specify maximum of 24h to be scrapped to avoid accidents
- (NOT IMPLEMENTED YET) If counts does not equal `scrap`, e.g. the count is 5 but only 2 more need to be scrapped, it will scrap exactly 2. Currently it would ignore these 2. see also #125
- (NOT IMPLEMENTED YET) If no counts are available for this asset, but uniqueProducts are available, they can also be marked as scrap. //TODO

{{%/alert%}}

`scrap` in the JSON is an integer.

| key | data type | description |
|-----|-----|--------------------------|
| `scrap` | int | Number of item from `count` that is considered as `scrap`. When `scrap` is equal to 0, that means all produced goods are good quality |

{{% alert title="Example for /scrapCount" color="primary" %}}
```json
{
    "timestamp_ms": 1588879689394, 
    "scrap": 1
}
```
{{%/alert%}}

### /barcode

Topic: `ia/<customerID>/<location>/<AssetID>/barcode`

| key | data type | description |
|-----|-----|--------------------------|
| `barcode` | str | A message is sent here each time the barcode scanner connected to the transmitter via USB reads a barcode via `barcodescanner` |

{{% alert title="Example for /barcode" color="primary" %}}
```json
{
    "timestamp_ms": 1588879689394, 
    "barcode": "16699"
}
```
{{%/alert%}}

### /activity

Topic: `ia/<customerID>/<location>/<AssetID>/activity`

| key | data type | description |
|-----|-----|--------------------------|
| `activity` | bool | A message is sent here every time the machine runs or stops (independent whether it runs slow or fast, or which reason the stop has. This is covered in [state](#state)) |

{{% alert title="Example for /activity" color="primary" %}}
```json
{
    "timestamp_ms": 1588879689394, 
    "activity": True
}
```
{{%/alert%}}

### /detectedAnomaly

Topic: `ia/<customerID>/<location>/<AssetID>/detectedAnomaly`

| key | data type | description |
|-----|-----|--------------------------|
| `detectedAnomaly` | str | A message is sent here each time a stop reason has been identified automatically or by input from the machine operator (independent whether it runs slow or fast, or which reason the stop has. This is covered in [state](#state)) |

{{% alert title="Example for /detectedAnomaly" color="primary" %}}
```json
{
    "timestamp_ms": 1588879689394, 
    "detectedAnomaly": "maintenance"
}
```
{{%/alert%}}

### /addShift

Topic: `ia/<customerID>/<location>/<AssetID>/addShift`

| key | data type | description |
|-----|-----|--------------------------|
| `timestamp_ms_end` | int | A message is sent here each time a new shift is started. The value represents a UNIX timestamp in milliseconds |

{{% alert title="Example for /addShift" color="primary" %}}
```json
{
    "timestamp_ms": 1588879689394, 
    "timestamp_ms_end": 1588879689395
}
```
{{%/alert%}}

### /addOrder

Topic: `ia/<customerID>/<location>/<AssetID>/addOrder`

A message is sent here each time a new order is started.

| key | data type | description |
|-----|-----|--------------------------|
| [`product_id`](/docs/concepts/mqtt/#explanation-of-ids) | str | Represents the current product name |
| [`order_id`](/docs/concepts/mqtt/#explanation-of-ids) | str | Represents the current order name |
| `target_units` | int | Represents the amount of target units to be produced (in the same unit as [count](#count)) |

{{% alert title="Attention" color="warning" %}}
1. The product needs to be added before adding the order. Otherwise, this message will be discarded
2. One order is always specific to that asset and can, by definition, not be used across machines. For this case one would need to create one order and product for each asset (reason: one product might go through multiple machines, but might have different target durations or even target units, e.g. one big 100m batch get split up into multiple pieces)
{{%/alert%}}

{{% alert title="Example for /addOrder" color="primary" %}}
```json
{
    "product_id": "Beierlinger 30x15",
    "order_id": "HA16/4889",
    "target_units": 1
}
```
{{%/alert%}}

### /addProduct

Topic: `ia/<customerID>/<location>/<AssetID>/addProduct`

A message is sent here each time a new product is added.

| key | data type | description |
|-----|-----|--------------------------|
| [`product_id`](/docs/concepts/mqtt/#explanation-of-ids) | str | Represents the current product name |
| `time_per_unit_in_seconds` | float | Specifies the target time per unit in seconds |

{{% alert title="Attention" color="warning" %}}
See also notes regarding adding products and orders in [/addOrder](#addorder)
{{%/alert%}}

{{% alert title="Example for /addProduct" color="primary" %}}
```json
{
    "product_id": "Beierlinger 30x15",
    "time_per_unit_in_seconds": 0.2
}
```
{{%/alert%}}

### /startOrder

Topic: `ia/<customerID>/<location>/<AssetID>/startOrder`

A message is sent here each time a new order is started.

| key | data type | description |
|-----|-----|--------------------------|
| [`order_id`](/docs/concepts/mqtt/#explanation-of-ids) | str | Represents the order name |


{{% alert title="Attention" color="warning" %}}
1. See also notes regarding adding products and orders in [/addOrder](#addorder)
2. When startOrder is executed multiple times for an order, the last used timestamp is used.
{{%/alert%}}

{{% alert title="Example for /startOrder" color="primary" %}}
```json
{
    "timestamp_ms": 1588879689394,
    "order_id": "HA16/4889",
}
```
{{%/alert%}}

### /endOrder

Topic: `ia/<customerID>/<location>/<AssetID>/endOrder`

A message is sent here each time a new order is started.

| key | data type | description |
|-----|-----|--------------------------|
| [`order_id`](/docs/concepts/mqtt/#explanation-of-ids) | str | Represents the order name |

{{% alert title="Attention" color="warning" %}}
1. See also notes regarding adding products and orders in [/addOrder](#addorder)
2. When endOrder is executed multiple times for an order, the last used timestamp is used.
{{%/alert%}}

{{% alert title="Example for /endOrder" color="primary" %}}
```json
{
    "timestamp_ms": 1588879689394,
    "order_id": "HA16/4889",
}
```
{{%/alert%}}

### /processValue

Topic: `ia/<customerID>/<location>/<AssetID>/processValue`

A message is sent here every time a process value has been prepared. Unique naming of the key.

| key | data type | description |
|-----|-----|--------------------------|
| `<valueName>` | int or float | Represents a process value, e.g. temperature.|

{{% alert title="Attention" color="warning" %}}
As `<valueName>` is a integer or float, booleans like "true" or "false" are not possible. Please convert them to integer, e.g., "True" --> 1, "False" --> 0
{{%/alert%}}

{{% alert title="Example for /processValue" color="primary" %}}
```json
{
    "timestamp_ms": 1588879689394, 
    "energyConsumption": 123456
}
```
{{%/alert%}}

### /productImage

Topic structure: `ia/<customer>/<location>/<assetID>/productImage`

`/productImage` has the same data format as [`ia/rawImage`](#topic-iarawimage), only with a changed topic.

`/productImage` can be acquired in two ways, either from [`ia/rawImage`](#topic-iarawimage) or `/rawImageClassification`. In the case of `/rawImageClassification`,
only the Image part is extracted to `/productImage`, while the classification information is stored in the relational database.

{{< imgproc imageProcessing Fit "451x522" >}}{{< /imgproc >}}

| key | data type | description |
|-----|-----|--------------------------|
|`image_id` | str | a unique identifier for every image acquired (e.g. format:`<MACaddress>_<timestamp_ms>`) |
|`image_bytes` | str | base64 encoded image in JPG format in bytes |
|`image_height` | int | height of the image in pixel |
|`image_width` | int | width of the image in pixel |
|`image_channels` | int | amount of included color channels (Mono: 1, RGB: 3) |


{{% alert title="Example for /productImage" color="primary" %}}
```json
{
	"timestamp_ms": 214423040823,
	"image":  {
		"image_id": "<SerialNumberCamera>_<timestamp_ms>",
		"image_bytes": "3495ask484...",
		"image_heigth": 800,
		"image_width": 1203,
		"image_channels": 3
	}
}
```
{{%/alert%}}

### /productTag

Topic structure: `ia/<customer>/<location>/<assetID>/productTag`

`/productTag` is usually generated by contextualizing a processValue to a product. 

| key | data type | description |
|-----|-----|--------------------------|
|`AID` | str | lorem ipsum |
|`name` | str | lorem ipsum |
|`value` | int | lorem ipsum |

{{% alert title="Example for /productTag" color="primary" %}}
```json
{
    "timestamp_ms": 215348452385,
    "AID": "14432504350",
    "name": "torque",
    "value": 2.12
}
```
{{%/alert%}}

**See also [Digital Shadow](/docs/concepts/digitalshadow/) for more information how to use this message**

### /productTagString

Topic structure: `ia/<customer>/<location>/<assetID>/productTagString`

`/productTagString` is usually generated by contextualizing a processValue to a product. 

| key | data type | description |
|-----|-----|--------------------------|
|`AID` | str | lorem ipsum |
|`name` | str | lorem ipsum |
|`value` | str | lorem ipsum |

{{% alert title="Example for /productTagString" color="primary" %}}
```json
{
    "timestamp_ms": 1243204549,
    "AID": "32493855304",
    "name": "QualityClass",
    "value": "Quality2483"
}
```
{{%/alert%}}

**See also [Digital Shadow](/docs/concepts/digitalshadow/) for more information how to use this message**

### /addParentToChild

Topic structure: `ia/<customer>/<location>/<assetID>/addParentToChild`

`/productTagString` is usually generated whenever a product is transformed into another product. It can be used multiple times for the same child to model that one product can consists out of multiple parents.

| key | data type | description |
|-----|-----|--------------------------|
|`childAID` | str | The AID of the child |
|`parentAID` | str | The AID of the parent |

{{% alert title="Example for /productTagString" color="primary" %}}
```json
{
    "timestamp_ms": 124387,
    "childAID": "23948723489",
    "parentAID": "4329875"
}
```
{{%/alert%}}

**See also [Digital Shadow](/docs/concepts/digitalshadow/) for more information how to use this message**

## 3rd level: production data

This level contains only highly aggregated production data.

### /state

Topic: `ia/<customerID>/<location>/<AssetID>/state`

A message is sent here each time the asset changes status. Subsequent changes are not possible. Different statuses can also be process steps, such as "setup", "post-processing", etc. You can find a list of all supported states [here](/docs/concepts/state/ )

| key | data type | description |
|-----|-----|--------------------------|
| `state` | int | Value of state according to [this datamodel](/docs/concepts/state/ ) |

{{% alert title="Example for /state" color="primary" %}}
```json
{
    "timestamp_ms": 1588879689394, 
    "state": 10000
}
```
{{%/alert%}}

### /cycleTimeTrigger

Topic: `ia/<customerID>/<location>/<AssetID>/cycleTimeTrigger`

A message should be sent under this topic whenever an assembly cycle is started.

`currentStation` in the JSON is a string
`lastStation` in the JSON is a string
`sanityTime_in_s` in the JSON is a integer

| key | data type | description |
|-----|-----|--------------------------|
| `currentStation` | str |  |
| `lastStation` | str |  |
| `sanityTime_in_s` | int |  |

{{% alert title="Example for /cycleTimeTrigger" color="primary" %}}
```json
{
    "timestamp_ms": 1611170736684,
    "currentStation": "1a",
    "lastStation": "1b",
    "sanityTime_in_s": 100
}
```
{{%/alert%}}

### /uniqueProduct

Topic: `ia/<customerID>/<location>/<AssetID>/uniqueProduct`

A message is sent here each time a product has been produced or modified. A modification can take place, for example, due to a downstream quality control.

There are two cases of when to send a message under the `uniqueProduct` topic:

- The exact product doesn't already have a UID (-> This is the case, if it has not been produced at an asset 
incorporated in the digital shadow). Specify a space holder asset = "storage" in the MQTT message for the 
`uniqueProduct` topic.
- The product was produced at the current asset (it is now different from before, e.g. after machining or after 
something was screwed in). The newly produced product is always the "child" of the process. Products it was made 
out of are called the "parents". 

| key | data type | description |
|-----|-----|--------------------------|
| `begin_timestamp_ms` | int | Start time |
| `end_timestamp_ms` | int | Completion time |
| [`product_id`](/docs/concepts/mqtt/#explanation-of-ids) | str | The product ID that is currently produced |
| `isScrap` | bool | Information whether the current product is of poor quality and will be sorted out. Default value (if not specified otherwise) is `false` |
| [`uniqeProductAlternativeID`](/docs/concepts/mqtt/#explanation-of-ids) | str | lorem ipsum |

{{% alert title="Example for /uniqueProduct" color="primary" %}}
```json
{
  "begin_timestamp_ms": 1611171012717,
  "end_timestamp_ms": 1611171016443,
  "product_id": "test123",
  "is_scrap": false,
  "uniqueProductAlternativeID": "12493857-a"
}
```
{{%/alert%}}

**See also [Digital Shadow](/docs/concepts/digitalshadow/) for more information how to use this message**

### /scrapUniqueProduct

Topic: `ia/<customerID>/<location>/<AssetID>/scrapUniqueProduct`

A message is sent here each time a unique product has been scrapped.

| key | data type | description |
|-----|-----|--------------------------|
| `UID` | str | Unique ID of the current single product |

{{% alert title="Example for /scrapUniqueProduct" color="primary" %}}
```json
{
    "UID": "161117101271788647991611171016443"
}
```
{{%/alert%}}

## 4th level: Recommendations for action

### /recommendations

Topic: `ia/<customerID>/<location>/<AssetID>/recommendations`

Shopfloor insights are recommendations for action that require concrete and rapid action in order to quickly eliminate efficiency losses on the store floor.

| key | data type/format | description |
|-----|-----|--------------------------|
| `recommendationUID` | int | Unique ID of the recommendation. Used to subsequently deactivate a recommendation (e.g. if it has become obsolete) |
| `recommendationType` | int | The ID / category of the current recommendation. Used to narrow down the group of people |
| `recommendationValues` | dict | Values used to form the actual recommendation set |

{{% alert title="Example for /recommendations" color="primary" %}}
```json
{
    "timestamp_ms": 1588879689394,
    "recommendationUID": 3556,
    "recommendationType": 8996,
    "enabled": True,
    "recommendationValues": 
    {
        "percentage1": 30, 
        "percentage2": 40
    }
}
```
{{%/alert%}}

## Explanation of IDs

There are various different IDs that you can find in the MQTT messages. This section is designed to give an overview.


| Name | data type/format | description |
|-----|-----|--------------------------|
| [`product_id`](/docs/concepts/mqtt/#explanation-of-ids) | int | The type of product that should be produced in an order for a specific asset. Can be used to retrieve the target speed. |
| [`order_id`](/docs/concepts/mqtt/#explanation-of-ids) | int | Order ID, which provides the type of product (see [`product_id`](/docs/concepts/mqtt/#explanation-of-ids)) and the amount of pieces that should be produced. |
| [`uniqeProductAlternativeID`](/docs/concepts/mqtt/#explanation-of-ids) | int | In short: AID. Used to describe a single product of the type [`product_id`](/docs/concepts/mqtt/#explanation-of-ids) in the order [`order_id`](/docs/concepts/mqtt/#explanation-of-ids). This is the ID that might be written on the product (e.g., with a physical label, lasered, etc.) and is usually the relevant ID for engineers and for production planning. It usually stays the same. |
| `UID` | int | Short for unique product ID. Compared to the AID the UID changes whenever a product changes its state. Therefore, a product will change its UID everytime it is placed on a new asset. It is used mainly on the database side to lookup a specific product in a specific state. |

These IDs are linked together in the database.

## TimescaleDB structure

Here is a scheme of the timescaleDB structure:
{{< imgproc timescaleDB Fit "1792x950" >}}{{< /imgproc >}}

*(open the image using the right click for a better resolution)*


[sensorconnect]: /docs/developers/factorycube-edge/sensorconnect
[cameraconnect]: /docs/developers/factorycube-edge/cameraconnect
