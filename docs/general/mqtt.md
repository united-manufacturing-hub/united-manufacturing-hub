
# The United datamodel / MQTT messages

## Contents

- [The United datamodel / MQTT messages](#the-united-datamodel--mqtt-messages)
  - [Contents](#contents)
  - [Introduction](#introduction)
  - [1st level: Raw data](#1st-level-raw-data)
    - [Topic: ia/raw/](#topic-iaraw)
      - [Example for ia/raw/](#example-for-iaraw)
  - [2nd level: contextualized data](#2nd-level-contextualized-data)
    - [/count](#count)
      - [Example for /count](#example-for-count)
    - [/barcode](#barcode)
      - [Example for /barcode](#example-for-barcode)
    - [/activity](#activity)
      - [Example for /activity](#example-for-activity)
    - [/detectedAnomaly](#detectedanomaly)
      - [Example for /detectedAnomaly](#example-for-detectedanomaly)
    - [/addShift](#addshift)
      - [Example for /addShift](#example-for-addshift)
    - [/addOrder](#addorder)
      - [Example for /addOrder](#example-for-addorder)
    - [/addProduct](#addproduct)
      - [Example for /addProduct](#example-for-addproduct)
    - [/startOrder](#startorder)
      - [Example for /startOrder](#example-for-startorder)
    - [/endOrder](#endorder)
      - [Example for /endOrder](#example-for-endorder)
    - [/processValue](#processvalue)
      - [Example for /processValue](#example-for-processvalue)
  - [3rd level: production data](#3rd-level-production-data)
    - [/state](#state)
      - [Example for /state](#example-for-state)
    - [/cycleTimeTrigger](#cycletimetrigger)
      - [Example for /cycleTimeTrigger](#example-for-cycletimetrigger)
    - [/uniqueProduct](#uniqueproduct)
      - [Example for /uniqueProduct](#example-for-uniqueproduct)
    - [/scrapUniqueProduct](#scrapuniqueproduct)
      - [Example for /scrapUniqueProduct](#example-for-scrapuniqueproduct)
  - [4th level: Recommendations for action](#4th-level-recommendations-for-action)
    - [/recommendations](#recommendations)
      - [Example for /recommendations](#example-for-recommendations)
  - [in development](#in-development)
    - [/qualityClass](#qualityclass)
    - [/detectedObject](#detectedobject)
    - [/cycleTimeTrigger](#cycletimetrigger-1)
    - [/cycleTimeScrap](#cycletimescrap)

## Introduction

All events or subsequent changes in production are transmitted via MQTT in the following data model. This ensures that all participants are always informed about the latest status.

The data model in the MQTT Broker can be divided into four levels. In general, the higher the level, the lower the data frequency and the more the data is prepared.

If you do not know the idea of MQTT (important keywords: "broker", "subscribe", "publish", "topic"), we recommend reading the [wikipedia article](https://en.wikipedia.org/wiki/MQTT) first.

All MQTT messages consist out of one JSON with atleast two elements in it.

1. `timestamp_ms`: the amount of milliseconds since the 1970-01-01 (also called [UNIX timestamp](https://en.wikipedia.org/wiki/Unix_time) in milliseconds)
2. `<valueName>`: a value

Some messages might deviate from it, but this will be noted explicitly. All topics are to be written in lower case only!

## 1st level: Raw data

Here are all raw data, which are not yet contextualized, i.e. assigned to a machine. These are in particular all data from [sensorconnect].

### Topic: ia/raw/

All raw data coming in via [sensorconnect].

Topic structure: `ia/raw/<transmitterID>/<gatewaySerialNumber>/<portNumber>/<IOLinkSensorID>`

#### Example for ia/raw/

Topic: `ia/raw/2020-0102/0000005898845/X01/210-156`

This means that the transmitter with the serial number `2020-0102` has one ifm gateway connected to it with the serial number `0000005898845`. This gateway has the sensor `210-156` connected to the first port `X01`.

```json
{
"timestamp_ms": 1588879689394, 
"distance": 16
}
```

## 2nd level: contextualized data

In this level the data is already assigned to a machine.

Topic structure: `ia/<customerID>/<location>/<AssetID>/<Measurement>` e.g. `ia/dccaachen/aachen/demonstrator/count`.

An asset can be a machine, plant or line (Explicitly not a single station of an assembly cell).

By definition all topic names should be lower case only!

### /count

Topic: `ia/<customerID>/<location>/<AssetID>/count`

Here a message is sent every time something has been counted. This can be, for example, a good product or scrap.

`count` in the JSON is an integer.
`scrap` in the JSON is an integer, which is optional. It means `scrap` pieces of `count` are scrap. If not specified it is 0 (all produced goods are good).

#### Example for /count

```json
{
    "timestamp_ms": 1588879689394, 
    "count": 1
}
```

### /barcode

Topic: `ia/<customerID>/<location>/<AssetID>/barcode`

A message is sent here each time the barcode scanner connected to the transmitter via USB reads a barcode via `barcodescanner`.

`barcode` in the JSON is a string.

#### Example for /barcode

```json
{
    "timestamp_ms": 1588879689394, 
    "barcode": "16699"
}
```

### /activity

Topic: `ia/<customerID>/<location>/<AssetID>/activity`

A message is sent here every time the machine runs or stops (independent whether it runs slow or fast, or which reason the stop has. This is covered in [state](#state))

`activity` in the JSON is a boolean.

#### Example for /activity

```json
{
    "timestamp_ms": 1588879689394, 
    "activity": True
}
```

### /detectedAnomaly

Topic: `ia/<customerID>/<location>/<AssetID>/detectedAnomaly`

A message is sent here each time a stop reason has been identified automatically or by input from the machine operator (independent whether it runs slow or fast, or which reason the stop has. This is covered in [state](#state)).

`detectedAnomaly` in the JSON is a string.

#### Example for /detectedAnomaly

```json
{
    "timestamp_ms": 1588879689394, 
    "detectedAnomaly": "maintenance"
}
```

### /addShift

Topic: `ia/<customerID>/<location>/<AssetID>/addShift`

A message is sent here each time a new shift is started.

`timestamp_ms_end` in the JSON is a integer representing a UNIX timestamp in milliseconds.

#### Example for /addShift

```json
{
    "timestamp_ms": 1588879689394, 
    "timestamp_ms_end": 1588879689395
}
```

### /addOrder

Topic: `ia/<customerID>/<location>/<AssetID>/addOrder`

A message is sent here each time a new order is started.

`product_id` in the JSON is a string representing the current product name.
`order_id` in the JSON is a string representing the current order name.
`target_units` in the JSON is a integer and represents the amount of target units to be produced (in the same unit as [count](#count)).

**Attention:**

1. the product needs to be added before adding the order. Otherwise, this message will be discarded
2. one order is always specific to that asset and can, by definition, not be used across machines. For this case one would need to create one order and product for each asset (reason: one product might go through multiple machines, but might have different target durations or even target units, e.g. one big 100m batch get split up into multiple pieces)

#### Example for /addOrder

```json
{
    "product_id": "Beierlinger 30x15",
    "order_id": "HA16/4889",
    "target_units": 1
}
```

### /addProduct

Topic: `ia/<customerID>/<location>/<AssetID>/addProduct`

A message is sent here each time a new product is added.

`product_id` in the JSON is a string representing the current product name.
`time_per_unit_in_seconds` in the JSON is a float specifying the target time per unit in seconds.

**Attention:** See also notes regarding adding products and orders in [/addOrder](#addorder)

#### Example for /addProduct

```json
{
    "product_id": "Beierlinger 30x15",
    "time_per_unit_in_seconds": 0.2
}
```

### /startOrder

Topic: `ia/<customerID>/<location>/<AssetID>/startOrder`

A message is sent here each time a new order is started.

`order_id` in the JSON is a string representing the order name.

**Attention:**

1. See also notes regarding adding products and orders in [/addOrder](#addorder)
2. When startOrder is executed multiple times for an order, the last used timestamp is used.

#### Example for /startOrder

```json
{
    "timestamp_ms": 1588879689394,
    "order_id": "HA16/4889",
}
```

### /endOrder

Topic: `ia/<customerID>/<location>/<AssetID>/endOrder`

A message is sent here each time a new order is started.

`order_id` in the JSON is a string representing the order name.

**Attention:**

1. See also notes regarding adding products and orders in [/addOrder](#addorder)
2. When endOrder is executed multiple times for an order, the last used timestamp is used.

#### Example for /endOrder

```json
{
"timestamp_ms": 1588879689394,
"order_id": "HA16/4889",
}
```

### /processValue

Topic: `ia/<customerID>/<location>/<AssetID>/processValue`

A message is sent here every time a process value has been prepared. Unique naming of the key.

`<valueName>` in the JSON is a integer or float representing a process value, e.g. temperature.

#### Example for /processValue

```json
{
    "timestamp_ms": 1588879689394, 
    "energyConsumption": 123456
}
```

## 3rd level: production data

This level contains only highly aggregated production data.

### /state

Topic: `ia/<customerID>/<location>/<AssetID>/state`

A message is sent here each time the asset changes status. Subsequent changes are not possible. Different statuses can also be process steps, such as "setup", "post-processing", etc. You can find a list of all supported states [here](state.md)

`state` in the JSON is a integer according to [this datamodel](state.md)

#### Example for /state

```json
{
    "timestamp_ms": 1588879689394, 
    "state": 10000
}
```

### /cycleTimeTrigger

Topic: `ia/<customerID>/<location>/<AssetID>/cycleTimeTrigger`

A message should be sent under this topic whenever an assembly cycle is started.

`currentStation` in the JSON is a string
`lastStation` in the JSON is a string
`sanityTime_in_s` in the JSON is a integer

#### Example for /cycleTimeTrigger

```json
{
  "timestamp_ms": 1611170736684,
  "currentStation": "1a",
  "lastStation": "1b",
  "sanityTime_in_s": 100
}
```

### /uniqueProduct

Topic: `ia/<customerID>/<location>/<AssetID>/uniqueProduct`

A message is sent here each time a product has been produced or modified. A modification can take place, for example, due to a downstream quality control.

`UID`: Unique ID of the current single product.
`isScrap`: Information whether the current product is of poor quality and will be sorted out
`productID`: the product that is currently produced,
`begin_timestamp_ms`: Start time
`end_timestamp_ms`: Completion time
`stationID`: If the asset has several stations, you can also classify here at which station the product was created (optional).

#### Example for /uniqueProduct

```json
{
  "begin_timestamp_ms": 1611171012717,
  "end_timestamp_ms": 1611171016443,
  "productID": "test123",
  "UID": "161117101271788647991611171016443",
  "isScrap": false,
  "stationID": "1a"
}
```

### /scrapUniqueProduct

Topic: `ia/<customerID>/<location>/<AssetID>/scrapUniqueProduct`

A message is sent here each time a unique product has been scrapped.

`UID`: Unique ID of the current single product.

#### Example for /scrapUniqueProduct

```json
{
  "UID": "161117101271788647991611171016443",
}
```

## 4th level: Recommendations for action

### /recommendations

Topic: `ia/<customerID>/<location>/<AssetID>/recommendations`

Shopfloor insights are recommendations for action that require concrete and rapid action in order to quickly eliminate efficiency losses on the store floor.

`recommendationUID`: Unique ID of the recommendation. Used to subsequently deactivate a recommendation (e.g. if it has become obsolete).
`recommendationType`: The ID / category of the current recommendation. Used to narrow down the group of people
`recommendationValues`: Values used to form the actual recommendation set

#### Example for /recommendations

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

## in development

### /qualityClass

A message is sent here each time a product is classified. Example payload:
> **qualityClass 0 and 1 are defined by default.
> {.is-warning}

| qualityClass | Name | Description | Color under which this "State" is automatically visualized by the traffic light|
|---------|------|------------------|------------------|
| 0 | Good | The product does not meet the quality requirements | Green |
| 1 | Bad |The product does not meet the quality requirements| Red |

> **The qualityClass 2 and higher are freely selectable**.
> {.is-warning}

| qualityClass | Name | Description | Color under which this "State" is automatically visualized by the traffic light|
|---------|------|------------------|------------------|
| 2 | Cookie center broken |Cookie center broken| Freely selectable |
| 3 | Cookie has a broken corner |Cookie has a broken corner | Freely selectable |

```
{
"timestamp_ms": 1588879689394, 
"qualityClass": 1
}
```

### /detectedObject

> **in progress (Patrick)**
{.is-danger}

Under this topic, a detected object is published from the object detection. Each object is enclosed by a rectangular field in the image. The position and dimensions of this field are stored in rectangle. The type of detected object can be retrieved with the keyword object. Additionally, the prediction accuracy for this object class is given as confidence. The requestID is only used for traceability and assigns each recognized object to a request/query, i.e. to an image. All objects with the same requestID were detected in one image capture.

```
{
"timestamp_ms": 1588879689394, 
}, "detectedObject": 
 {
   "rectangle":{
    "x":730,
    "y":66,
    "w":135,
    "h":85
   },
   { "object": "fork",
   "confidence":0.501
  },
"requestID":"a7fde8fd-cc18-4f5f-99d3-897dcd07b308"
}
```

### /cycleTimeTrigger

A message should be sent under this topic whenever an assembly cycle is started.

```
{
"timestamp_ms" : 1588879689394,
"currentStation" : "StationXY",
"nextStation" : "StationYZ",
"sanityTime_in_s": 12 // time after current cycle should be aborted because it takes unrealistically long time (in seconds).
}
```

### /cycleTimeScrap

Under this topic a message should be sent whenever an assembly at a certain station should be aborted because the part has been marked as defective.

```
{ 
"timestamp_ms" : 1588879689394,
"currentStation" : "StationXY"
}
```

[sensorconnect]: ../edge/sensorconnect.md
