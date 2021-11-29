---
title: "Postgres"
linkTitle: "Postgres"
weight: 2
description: >
    The following model documents our internal postgres tables
---

## TimescaleDB structure

Here is a scheme of the timescaleDB structure:
[{{< imgproc database-model Fit "1792x950" >}}{{< /imgproc >}}](database-model.png)



### AvailabilityLossState
This table holds state ids for availability losses
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `Value` | bigint | Open [available states for assets](https://docs.umh.app/docs/concepts/state/) for more information 
| `Name` | text | Name of AvailabilityLossState


### AvailabilityLossStates
This table is a many-to-many relation table between StationConfigurations and AvailabilityLossState
| key | data type/format | description |
|-----|-----|--------------------------|
| `StationConfigurationId` | int | References [StationConfiguration.Id](#stationconfiguration) 
| `AvailabilityLossStateId` | int | References [AvailabilityLossState.Id](#availabilitylossstate) 
#### Indices
| keys | type |
|-----|-----|
| `StationConfigurationId`, `AvailabilityLossStateId` | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)


### Count
This table shows the count of products produced at a specific timestamp

SELECT create_hypertable("Count", "Timestamp");

CREATE INDEX ON Count (StationStep, Timestamp DESC);
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `StationStepId` | int | References [StationStep.Id](#stationstep) 
| `Count` | bigint | Count of products produced at specific timestamp 
| `Timestamp` | timestamp | Timestamp of Count creation
#### Indices
| keys | type |
|-----|-----|
| `StationStepId`, `Timestamp` | [Unique](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-UNIQUE-CONSTRAINTS)


### Location
This table holds physical locations of Stations
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `Name` | text | Name of Location


### Order
This table holds an order for a station

It holds the order name and the target unit count

EndTimestamp is allowed to be null here, since we might not now, how long we want to produce

CHECK (BeginTimestamp < EndTimestamp)

CHECK (TargetUnits > 0)

EXCLUDE USING gist (StationId WITH =, tstzrange(BeginTimestamp, EndTimestamp) WITH &&) WHERE (BeginTimestamp IS NOT NULL AND EndTimestamp IS NOT NULL)
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `StationId` | int | References [Station.Id](#station) 
| `Name` | text | Name of Order
| `BeginTimestamp` | timestamp | Begin of order
| `EndTimestamp` | Option<timestamp> | End of order
| `TargetUnits` | bigint | Amount of [Products](#product) to produce 
#### Indices
| keys | type |
|-----|-----|
| `StationId`, `Id` | [Unique](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-UNIQUE-CONSTRAINTS)


### PerformanceLossState
This table holds state ids for performance losses
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `Value` | bigint | Open [available states for assets](https://docs.umh.app/docs/concepts/state/) for more information 
| `Name` | text | Name of PerformanceLossState


### PerformanceLossStates
This table is a many-to-many relation table between StationConfigurations and PerformanceLossState
| key | data type/format | description |
|-----|-----|--------------------------|
| `StationConfigurationId` | int | References [StationConfiguration.Id](#stationconfiguration) 
| `PerformanceLossStateId` | int | References [PerformanceLossState.Id](#performancelossstate) 
#### Indices
| keys | type |
|-----|-----|
| `StationConfigurationId`, `PerformanceLossStateId` | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)


### Process
Each StationStep is part of a process (for example: Install Engine)
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `Name` | text | Name of Process
| `CreationTime` | timestamp | UNIX creation timestamp of Process


### ProcessGroupRelations
Models the relation between Process and StationStep
| key | data type/format | description |
|-----|-----|--------------------------|
| `ProcessId` | int | References [Process.Id](#process) 
| `StationStepId` | int | References [StationStep.Id](#stationstep) 
#### Indices
| keys | type |
|-----|-----|
| `ProcessId`, `StationStepId` | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)


### ProcessValue
This table shows the ProcessValue for a StationStep (indirect) at a specific timestamp
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `ValueTypeId` | int | References [ProcessValueType.Id](#processvaluetype) 
| `Value` | double | Process value as double 
| `Timestamp` | timestamp | Timestamp of ProcessValue creation


### ProcessValueError
This table shows the ProcessValueError for a StationStep (indirect) at a specific timestamp
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `ValueTypeId` | int | References [ProcessValueType.Id](#processvaluetype) 
| `Error` | text | Process value error text 
| `Timestamp` | timestamp | Timestamp of ProcessValueError creation


### ProcessValueType
This table is a meta-table to reference a ProcessValue or ProcessValueError by a StationStep
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `StationStepId` | int | References [StationStep.Id](#stationstep) 
| `Name` | text | Name of ProcessValueType


### Product
This table holds a unique product

Each product references the StationStep it was produced

It also reference the ProductType by the StationProduct of its Station

CHECK: ProductFailureId can only be set if ProductStatus::Undefined
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `StationStepId` | int | References [StationStep.Id](#stationstep) 
| `StationProductId` | int | References [StationProduct.Id](#stationproduct) 
| `Value` | text | TODO: What does this value exactly represent ? 
| `ProductionBegin` | timestamp | UNIX timestamp of production begin 
| `ProductionEnd` | Option<timestamp> | UNIX timestamp of production end 
| `Status` | ProductStatus | References [ProductStatus](#productstatus) 
| `ProductFailureId` | Option<int> | References [ProductFailure.Id](#productfailure) 


### ProductFailure
Holds an Value, describing the ProductFailure further
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `Value` | text | Failure description 


### ProductInheritance
This table references the Parent product of another Product (and vise-versa)
| key | data type/format | description |
|-----|-----|--------------------------|
| `ParentId` | int | References [Product.Id](#product) 
| `ChildId` | int | References [Product.Id](#product) 
#### Indices
| keys | type |
|-----|-----|
| `ParentId`, `ChildId` | [Unique](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-UNIQUE-CONSTRAINTS)
| `ParentId` | 
| `ChildId` | 


### ProductStatus
| key | description |
|-----|--------------------------|
| NoScrap | means that the product was tested and good or that no testing is needed for this product
| Scrap | means that the product was tested and found to be bad
| Undefined | means that the status is not known, this could be a sensor error
### ProductTagDouble
This table models a tag of type double for a unique product
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `Timestamp` | timestamp | Timestamp of ProductTagDouble creation
| `ProductId` | int | References [Product.Id](#product) 
| `ValueName` | text | Name of ProductTagDouble
| `Value` | double | TODO: What exactly will be saved here ? 


### ProductTagString
This table models a tag of type string for a unique product
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `Timestamp` | timestamp | Timestamp of ProductTagString creation
| `ProductId` | int | References [Product.Id](#product) 
| `ValueName` | text | Name of ProductTagString
| `Value` | string | TODO: What exactly will be saved here ? 


### ProductTypeName
This table holds the name of a product type
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `Name` | text | Name of ProductTypeName


### RecommendationInstanceTable
This is an Instance of a recommendation for a specific station
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `StationId` | int | References [Station.Id](#station) 
| `RecommendationTemplateId` | int | References [RecommendationTemplate.Id](#recommendationtemplate) 
| `Timestamp` | timestamp | Timestamp of RecommendationInstanceTable creation


### RecommendationTemplate
This is a template for a recommendation

These can be instantiated.

TODO: Which of these keys should be not null

TODO maybe add table for translations
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `Timestamp` | timestamp | Timestamp of RecommendationTemplate creation
| `Type` | bigint | TODO: What exactly will be saved here ? 
| `Enabled` | boolean | Template is enabled 
| `Values` | Option<text> | TODO: What exactly will be saved here ? 
| `DiagnoseTextDE` | Option<text> | German diagnose text 
| `DiagnoseTextEN` | Option<text> | English diagnose text 
| `TextDE` | Option<text> | German text for operator 
| `TextEN` | Option<text> | English text for operator 


### Shift
This table holds shifts for a Station

It also references a ShiftType

It is nullable for EndTimestamp, since we dont know the end of a shift in real-world scenarios

CHECK (BeginTimestamp < EndTimestamp)

EXCLUDE USING gist (StationId WITH =, tstzrange(BeginTimestamp, EndTimestamp) WITH &&)
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `ShiftTypeId` | int | References [ShiftType.Id](#shifttype) 
| `StationId` | int | References [Station.Id](#station) 
| `BeginTimestamp` | timestamp | Begin of shift
| `EndTimestamp` | Option<timestamp> | End of shift
#### Indices
| keys | type |
|-----|-----|
| `BeginTimestamp`, `StationId` | [Unique](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-UNIQUE-CONSTRAINTS)


### ShiftType
This table holds a shift type

This could for example be a normal shift, or maintenance
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `Name` | text | Name of ShiftType


### State
This table shows the state of a StationStep at a specific timestamp

SELECT create_hypertable("State", "Timestamp");

CREATE INDEX ON State (StationStep, Timestamp DESC);
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `StationStepId` | int | References [StationStep.Id](#stationstep) 
| `State` | bigint | TODO: Should this reference a AvailabilityLossState or PerformanceLossState ? 
| `Timestamp` | timestamp | Timestamp of State creation
#### Indices
| keys | type |
|-----|-----|
| `StationStepId`, `Timestamp` | [Unique](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-UNIQUE-CONSTRAINTS)


### Station
This table models a Station

Example:

In a factory line, producing Cars a Station could be installing the engine
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `StationGroupId` | int | References [StationGroup.Id](#stationgroup) 
| `LocationId` | int | References [Location.Id](#location) 
| `Name` | text | Name of Station
| `CreationTime` | timestamp | UNIX creation timestamp of Station
#### Indices
| keys | type |
|-----|-----|
| `Name`, `LocationId` | [Unique](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-UNIQUE-CONSTRAINTS)


### StationConfiguration
This table holds a configuration for an StationStep
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `StationStepId` | int | References [StationStep.Id](#stationstep) 
| `MicrostopDurationInSeconds` | bigint | TODO: What does this value exactly represent ? 
| `IgnoreMicrostopUnderThisDurationInSeconds` | bigint | Stops under this seconds will be ignored 
| `MinimumRunningTimeInSeconds` | bigint | TODO: What does this value exactly represent ? 
| `ThresholdForNoShiftsConsideredBreakInSecond` | bigint | If no shifts are booked for more then this value, a break is assumed 
| `LowSpeedThresholdInPcsPerHour` | bigint | If the production speed is lower then this value, a Performance loss is assumed 
| `LanguageCode` | varcharacter(10) | TODO: lang code len 


### StationGroup
This models a group of stations.

For example a assembly line producing cars
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `Name` | text | Name of StationGroup
| `CreationTime` | timestamp | UNIX creation timestamp of StationGroup


### StationProduct
This table holds a product type that will be produced at a station
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `StationId` | int | References [Station.Id](#station) 
| `ProductTypeNameId` | int | References [ProductTypeName.Id](#producttypename) 
| `TimePerUnitInSeconds` | double |  


### StationRecommendationRelations
This references which RecommendationTemplates are valid for a Station
| key | data type/format | description |
|-----|-----|--------------------------|
| `StationId` | int | References [Station.Id](#station) 
| `RecommendationTemplateId` | int | References [RecommendationTemplate.Id](#recommendationtemplate) 
#### Indices
| keys | type |
|-----|-----|
| `RecommendationTemplateId`, `StationId` | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)


### StationStep
This tables models a single step at a Station.

At a station that installs an engine to a car, this could be screwing down the engine block to the car chassis
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `Name` | text | Name of StationStep
| `CreationTime` | timestamp | UNIX creation timestamp of StationStep


### StationStepRelations
A Station has smaller sub-steps, this is modeled here
| key | data type/format | description |
|-----|-----|--------------------------|
| `StationId` | int | References [Station.Id](#station) 
| `StationStepId` | int | References [StationStep.Id](#stationstep) 
#### Indices
| keys | type |
|-----|-----|
| `StationId`, `StationStepId` | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)


### StationStepSequence
This models, in which sequence the steps at a station are executed

Example: Loading, Processing Step 1, Processing Step 2, Unloading
| key | data type/format | description |
|-----|-----|--------------------------|
| `Id` | int | [Primary Key](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-PRIMARY-KEYS)
| `StationId` | int | References [Station.Id](#station) 
| `StationStepId` | int | References [StationStep.Id](#stationstep) 
| `SequenceNumber` | int | Higher sequence number means, the StationStep is later in the Stations process 
#### Indices
| keys | type |
|-----|-----|
| `StationId`, `StationStepId`, `SequenceNumber` | [Unique](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-UNIQUE-CONSTRAINTS)



