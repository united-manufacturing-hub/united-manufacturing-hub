---
title: "Digital Shadow - track and trace"
linkTitle: "Digital Shadow - track and trace"
description: >
  A system of features allowing tracking and tracing of individual parts through the production process. 
---

*Digital shadow is still in development and not yet deployable.*
## Introduction

**Goal:**
In order to gain detailed insight in the production process and into the produced products we needed a system of 
features to acquire and access information gained by scanners, sensors etc. This allows better quality assurance 
and enables production improvement.

**Solution:**
We send MQTT messages containing a timestamp with a single value like a scanned ID, a measured value etc. from our edge
devices to the MQTT broker and contextualize them with microservices. The gained data is pushed into a database by the 
`MQTT-to-postgres` microservice. After that `factoryinsight` provides an interface for formatted data providing maximal 
usability of BI-Tools like tableau. The data is made available to a Tableau-server via a MySQL Database.

## Overall Concept
{{< imgproc digitalShadowConcept Fit "9063x2130" >}}{{< /imgproc >}}
This is the overview of the digital shadow concept.

The following chapters are going through the concept from left to right (from the inputs of the digital shadow to the outputs).

### Data Input

Data can be sent as JSON in a MQTT message to the central MQTT broker. UMH recommends to stick to the data definition of 
[the UMH datamodel] for the topics and messages, but the implementation is client specific and can be modeled for the 
individual problem. The relevant input data for digital shadow is on Level 1 and Level 2.

#### Example 1, raw, Level 1 data:
Topic: `ia/rawBarcode/2020-0102/210-156`\
Topic structure: `ia/rawBarcode/<transmitterID>/<barcodeReaderID>`
```json
{
"timestamp_ms": 1588879689394, 
"barcode": "1284ABCtestbarcode"
}
```
#### Example 2, sensorValue, Level 2:
Topic: `ia/testCustomerID123/testLocationID123/testAssetID123/processValue`\
Topic structure: `ia/<customerID>/<location>/<AssetID>/processValue`
```json
{
"timestamp_ms": 1588879689394, 
"torque": 5.345
}
```


### Contextualization + Messages for MQTT-to-postgres
Now the information is available at the mqtt-broker and because of that to all subscribed services. But we still need to
contextualize the information, meaning: we want to link gained data to specific products, because right now we only have
asset specific values with timestamps.

First microservices should be used (stateless if possible) to convert messages under a `raw` topic into messages under 
`processValue` or `processValueString`. This typically only requires resending the message under the appropriate topic or 
breaking messages with multiple values apart into single ones.

There are four specific kinds of messages regarding the digital shadow which need to be sent to the the `MQTT-to-postgres`
function:

- **productTag topic:** MQTT Messages containing one specific datapoint for one asset (specified in the MQTT topic):
`ia/<customerID>/<location>/<AssetID>/productTag`
```json
{
"timestamp_ms": 215348452385,
"AID": "14432504350",
"name": "torque",
"value": 2.12
}
```
- **productTagString Topic:** Because we also want to send strings we need a MQTT topic for strings:
`ia/<customerID>/<location>/<AssetID>/productTagString`

```json
{
"timestamp_ms": 1243204549,
"aid": 32493855304,
"name": "QualityClass",
"value": "Quality2483"
}
```
- **addParentToChild:** To describe relations between products and states of those products in the production 
  `MQTT-to-postgres` expects MQTT messages under the topic: `ia/<customerID>/<location>/<AssetID>/addParentToChild`. One
  message always contains one child AID and one parent AID. This specifies what parent product was used to generate the 
  child.
```json
{
"timestamp_ms": 124387,
"childAID": "23948723489",
"parentAID": "4329875"
}
```
- **uniqueProduct:** To indicate the generation of a new product or a new product state send a MQTT message to `MQTT-to-postgres`
under the topic: `ia/<customerID>/<location>/<AssetID>/uniqueProduct`.
  There are two cases of when to send a message under the `uniqueProduct` topic:
  - The exact product doesn't already have a UID (-> This is the case, if it has not been produced at an asset 
  incorporated in the digital shadow). Specify a space holder asset = "storage" in the MQTT message for the 
    `uniqueProduct` topic.
  - The product was produced at the current asset/step (it is now different from before, e.g. after machining or after 
    something was screwed in). The newly produced product is always the "child" of the process. Products it was made 
    out of are called the "parents". 
```json
{
  "begin_timestamp_ms": 1611171012717,
  "end_timestamp_ms": 1611171016443,
  "product_id": "test123",
  "is_scrap": false,
  "step_id": "1a",
  "uniqueProductAlternativeID": "12493857-a"
}
```

#### Generating the contextualized messages
The goal is to convert messages under the `processValue` and the `processValueString` topics, containing all relevant data,
into messages under the topic `productTag`, `productTagString` and `addParentToChild`. The latter messages contain AID's
which hold the contextualization information - they are tied to a single product.

The implementation of the generation of the above mentioned messages with contextualized information is up to the user 
and depends heavily on the specific process. To help with this we want to present a general logic and talk about the 
advantages and disadvantages of it:


#### General steps:
1. Make empty containers for predefined messages to `MQTT-to-postgres` when the first production step took place
2. Fill containers step by step when relevant messages come in.
3. If full, send the container.
4. If the message from the first production step for the new product is received before the container is full, 
   send container and set missing fields to null. Also send an error message.

#### Example process:
1. parent ID 1 scanned(specifically the later explained [AID](#uniqueProductAlternativeID)) -> barcode sent under 
   `processValueString` topic
2. screws fixed -> torque `processValue` send
3. child AID scanned -> barcode `processValueString` send
4. parent AID 2 scanned -> barcode `processValueString` send

Example of generating a message under `productTagString` topic containing the measured torque value for the Example
process:
- when parent AID scanned: make empty container for message because scanning parent AID is first step
```json
{
"timestamp_ms": 
"AID":
"name": "toque",
"value":
}
```
- when torque value comes in: fill in value and timestamp
```json
{
"timestamp_ms": 13498435234,
"AID":
"name": "toque",
"value": 1.458
}
```
- when child AID comes in: fill it in:
```json
{
"timestamp_ms": 13498435234,
"AID": "34258349857",
"name": "toque",
"value": 1.458
}
```
Now the container is full: send it away. \
**Important:** always send the `uniqueProduct` message first and afterwards the messages for the related 
`productTag`/`productTagString` and messages on the `addParentToChild` topic.


#### Advantages and disadvantages of presented process
Pro | Con
--- | ---
simple | not stateless
general usability good |might need a lot of different containers if the number of e.g. `productTag` messages gets to big

### uniqueProductID and uniqueProductAlternativeID
At this point it makes sense to talk about uniqueProductID's and uniqueProductAlternativeID's, in short UID's and AID's.
The concept behind these different types of ID's is crucial to understand, if you want to understand the later presented 
datastructures. Neither UID nor AID are defining the type of a product; they identify a single product itself. The UID 
is generated for every state a product was/is in and is mainly important for the database. The AID on the other hand 
might be from a physical label, or a written product number. It is usually the relevant ID for engineers and for 
production planning. The physical labels stay the same after assembly (the same AID can be related to multiple different
UID's). If we have multiple labels on one part we can also choose one of them for the AID.

AID's and UID's are stored  in combination one-to-one in the uniqueProductTable (timescaleDB).

#### Definition of when to change the UID
If we can move a product from point "A" in the production to point "B" or back without causing problems from a process 
perspective, the UID of the product should stay the same. (For example if the product only gets transported between 
point "A" and "B").

If moving the object produces problems (e.g. moving a not yet tested object in the bin "tested products"), the object 
should have gotten a new UID on its regular way.

#### Example 1: Testing
Even though testing a product doesn't change the part itself, it changes its state in the production process:
- it gets something like a virtual "certificate"
- the value increases because of that

-> Make a new UID.

#### Example 2: Transport
Monitored Transport from China to Germany (This would be a significant distance: transport data would be useful to 
include into digital shadow)
- parts value increases
- transport is separately paid
- not easy to revert

-> Make a new UID
#### Life of a single UID
Type | creation UID   |   death UID
--- | --- | ---
without inheritance at creation | topic: `storage/uniqueProduct`   |   `/addParentToChild` (UID is parent)
with inheritance at creation | topic: `<asset>/uniqueProduct` + `addParentToChild` (UID is child)| `/addParentToChild` (UID is parent)

MQTT messages under the `productTag` topic should not be used to indicate transport of a part. If transport is relevant, 
change the UID (-> send a new MQTT message to `MQTT-to-postgres` under the `uniqueProduct` topic).

#### Example process to show the usage of AID's and UID's in the production:
{{< imgproc productIDExample Fit "2026x1211" >}}{{< /imgproc >}}
#### Explanation of the diagram:
Assembly Station 1:
- ProductA and ProductB are combined into ProductC
- Because ProductA and ProductB have not been "seen" by the digital shadow, they get a new UID and asset = "storage" 
  assigned (placeholder asset for unknown/unspecified origin).
- After ProductC is now produced it gets a new UID and as an asset, Assy1, because it is the child at Assembly Station 1
- The AID of the child can always be freely chosen out of the parent AID's. The AID of ProductA ("A") is a physical 
  label. Because ProductB doesn't have a physical Label, it gets a generated AID. For ProductC (child) we can now choose 
  either the AID from ProductA or from ProductB. Because "A" is a physical label, it   makes sense to use the AID of 
  ProductA.

Now the ProductC is transported to Assembly Station 2. Because it is a short transport, doesn't add value etc. we do not
need to produce a new UID after the transport of ProductA.

Assembly Station 2:
- ProductC stays the same (in the sense that it is keeping its UID before and after the transport), because of the easy
  transport. 
- ProductD is new and not produced at assembly station 2, so it gets asset = "storage" assigned
- ProductC and ProductD are combined into ProductE. ProductE gets a new UID. Both AID's are physical. We again freely 
  choose the AID we want to use (AID C was chosen, maybe because after the assembly of ProductC and ProductD, the AID 
  Label on ProductC is not accessible while the AID Label on the ProductD is).


Assembly Station 3:
- At Assembly Station ProductE comes in and is turned into ProductF
- ProductF gets a new UID and keeps the AID of ProductE. It now gets the Assy3 assigned as asset.

Note that the `uniqueProduct` MQTT message for ProductD would not be under the Topic of Assembly2 as asset but for 
example under storage. The convention is, that every part never seen by digital shadow "comes" from storage even 
though the UID and the related `uniqueProduct` message is created at the current station.

#### Batches of parts
If for example a batch of screws is supplied to one asset with only one datamatrix code (one AID) for all screws 
together, there will only be one MQTT message under the topic `uniqueProduct` created for the batch with one AID, a 
newly generated UID and with the default supply asset `storage`.
- The batch AID is then used as parent for a MQTT message under the topic `addParentToChild`.
  (-> mqtt-to-postgres will repeatedly fetch the same parent uid for the inheritanceTable)
- The batch AID only changes when new batch AID is scanned.

### MQTT-to-postgres
The `MQTT-to-postgres` microservice now uses the MQTT messages it gets from the broker and writes the information in the
database. The microservice is not use-case specific, so the user just needs to send it the correct MQTT messages.

`MQTT-to-postgres` now needs to generate UID's and save the information in the database, because the database uses UID's
to store and link all the generated data efficiently. Remember that the incoming MQTT messages are contextualized with AID's.

We can divide the task of `MQTT-to-postgres` in three (regarding the digital shadow):
- Use the MQTT message under the Topic **uniqueProduct** which gives us the AID and the Asset and make an entry in the
  uniqueProduct table containing the AID and a newly generated UID.
  1. Generate UID (with snowflake: https://en.wikipedia.org/wiki/Snowflake_ID)
  2. Store new UID and all data from `uniqueProduct` MQTT Message in the  `uniqueProductTable`

- Use **productTag and productTagString** topic MQTT messages. The AID and the AssetId is used to look for the uniqueProduct 
  the messages belong to. The value information is then stored with the UID in the TimescaleDB 
  1. Look in TimescaleDB, `uniqueProductTable` for the uniqueProduct with the same Asset and AID from the `productTag` 
     massage (the child)
  2. Get the UID when found from the child (that is why it is important to send the `uniqueProduct` message before sending
     `productTag`/`productTagString`).
  3. Write value information without AID, instead with the found UID in the uniqueProductTable

- Use the **addParentToChild** message. Retrieve the child UID by using the child AID and the Asset. Get the parent 
  UID's by finding the last time the parents AID's were stored in the uniqueProductTable.
  1. Look in TimescaleDB, `uniqueProductTable` for the uniqueProduct with the same Asset and AID as written in the child 
     of the /addParentToChild message
  2. Look in the TimescaleDB, `uniqueProductTable` for all other assets for the last time the AID of the parent was used 
     and get the UID
  3. Write UID of child and UID of the parent in the `productInheritanceTable`

**Possible Problems:**
- The `uniqueProduct` MQTT message of the child has to be made before we can store `productTag` or `productTagString`
  messages. 
- All `uniqueProducts` of one step at one asset need to be stored before we can process `addParentToChild` messages.
This means we also need to send possible parent `uniqueProduct` MQTT messages (asset = `storage`) before.

### Sql Database Structure (timescaleDB)
*The structure of the timescaleDB might be changed in the future.*
{{< imgproc timescaleDB Fit "2026x1211" >}}{{< /imgproc >}}
Four tables are especially relevant:
- `uniqueProductTable` contains entries with a pair of one UID and one AID and other data.
- `productTagTable` and `productTagStringTable` store information referenced to the UID's in the `uniqueProductTable`. 
  Stored is everything from individual measurements to quality classes.
- `productInheritanceTable` contains pairs of child and parent UID's. The table as a whole thereby contains the complete 
  inheritance information of each individual part. One entry describes one edge of the inheritance graph.

The new relevant tables are dotted, the `uniqueProductTable` changes are bold in the timescaleDB structure visualization. 
### Factoryinsight + Rest API
To make the relevant data from digital shadow available we need to provide new REST API's. `Factoryinsight` is the
microservice doing that task. It accepts specific requests, accesses the timescale database and returns 
the data in the desired format.

#### Implemented functionality for digital shadow
The following function returns all uniqueProducts for that specific asset and step_id in a specified time range. One datapoint contains one 
childUID, all parentUID's and all available alternativeUniqueProductID's. All uniqueProductTags and 
uniqueProductTagStrings (value and timestamp) for the childUID are returned to the same datapoint.

`get /{customer}/{location}/{asset}/uniqueProductsWithTags`
from `<timestamp1>` to `<timestamp2>` (in RFC 3999 Format) and for a specific `AssetID` and `step_id`.

Example Format: 
```json
[
   row

]

  row:
{
"ValveHeadAlternativeUniqueID": str<123>,
"TorqueScrew1": number<5.0>,
"TorqueScrew2": number<5.0>,
"TorqueScrew3": number<5.0>,
"TorqueScrew4": number<5.0>,
"gasketUID": number<5.0>,
"childUID": number<5.0>,
"Timestamp_ValveHeadID": number<5.0>,
"Timestamp_GasketImageID": number<5.0>
}
```


Example Return:
```json
[
  {
    "ValveHeadAlternativeUniqueID": "123",
    "TorqueScrew1": 5.2,
    "TorqueScrew2": 5.0,
    "TorqueScrew3": 5.0,
    "TorqueScrew4": 5.0,
    "gasketUID":5.0,
    "childUID": 5.0,
    "Timestamp_ValveHeadID": 5.0,
    "Timestamp_GasketImageID": 5.0
  },
  {
    "ValveHeadAlternativeUniqueID": "124",
    "TorqueScrew1" : 5.0,
    "TorqueScrew2" : 5.0,
    "TorqueScrew3" : 5.0,
    "TorqueScrew4" : 5.0,
    "gasketUID" : 5.0,
    "childUID" : 5.0,
    "Timestamp_ValveHeadID": 5.0,
    "Timestamp_GasketImageID": 5.0
  },
  {
    "ValveHeadAlternativeUniqueID": "125",
    "TorqueScrew1" : 5.0,
    "TorqueScrew2" : 5.0,
    "TorqueScrew3" : 5.0,
    "TorqueScrew4" : 5.0,
    "gasketUID" : 5.0,
    "childUID":  5.0,
    "Timestamp_ValveHeadID": 5.0,
    "Timestamp_GasketImageID": 5.0
  }

]
```
#### Implemented logic of factoryinsight to achieve the functionality
1. Get all productUID's and AID's from `uniqueProductTable` within the specified time and from the specified asset and station.
2. Get all parentUID's from the `productInheritanceTable` for each of the selected UID's.
3. Get the AID's for the parentUID's from the `uniqueProductTable`.
4. Get all key, value pairs from the `productTagTable` and `productTagStringTable` for the in step 1 selected UID's.
5. Return all parent AID's, the child UID and AID, all the productTag and all the productTagString values and timestamps.

### SQL Database to connect to Tableau server

For the digital shadow functionality we need to give the tableau server access to the data. Because the 
tableau server can't directly connect to the REST API, we need to either use a database in between, or a 
tableau web data connector. We were advised against the tableau web data connector 
(general info about tableau webdata connectors: 
https://help.tableau.com/current/pro/desktop/en-us/examples_web_data_connector.htm ). Because of that we implemented a 
sql database. We used MySQL because it is opensource, works well with node-red and with tableau, which makes it the best choice for
the task. 
According to the structure overview in the beginning of this article we are using node-red to fetch the required data
from the REST API of `factoryinsight` and push it into the MySQL database. The MySQL database can then be accessed by 
the tableau server.

## Industry Example
To test the digital shadow functionality and display its advantages we implemented the solution in a model factory.

{{< imgproc testProductionMQTT Fit "2822x2344" >}}{{< /imgproc >}}
This graphic displays the events and following MQTT messages, `MQTT-to-postgres` receives.



## Long term: planned features
We plan to integrate further functionalities to the digital shadow.
Possible candidates are:
- multiple new REST API's to use the digital shadow more flexible
- detailed performance analysis and subsequent optimization to enable digital shadow for massive 
  production speed and complexity
- A buffer in microservice `MQTT-to-postgres`. If `productTag`/`productTagString` messages are sent to the 
  microservice before writing the message `uniqueProduct` in the database the tags should be stored until the 
  `uniqueProduct` message arrives. A buffer could hold `productTag`/`productTagString` messages and regularly try to 
  write them in the database.


[the UMH datamodel]: ../../concepts/mqtt
[sensorconnect]: ../../developers/factorycube-core/sensorconnect
