# Documentation for factoryinsight

<a name="documentation-for-api-endpoints"></a>
## Documentation for API Endpoints

All URIs are relative to *https://api.industrial-analytics.net/api/v1*

Class | Method | HTTP request | Description
------------ | ------------- | ------------- | -------------
*DefaultApi* | [**getAssetAggregatedStates**](Apis/DefaultApi.md#getassetaggregatedstates) | **GET** /{customer}/{location}/{asset}/aggregatedStates | 
*DefaultApi* | [**getAssetAvailability**](Apis/DefaultApi.md#getassetavailability) | **GET** /{customer}/{location}/{asset}/availability | 
*DefaultApi* | [**getAssetAverageChangeoverTime**](Apis/DefaultApi.md#getassetaveragechangeovertime) | **GET** /{customer}/{location}/{asset}/averageChangeoverTime | 
*DefaultApi* | [**getAssetAverageCleaningTime**](Apis/DefaultApi.md#getassetaveragecleaningtime) | **GET** /{customer}/{location}/{asset}/averageCleaningTime | 
*DefaultApi* | [**getAssetCounts**](Apis/DefaultApi.md#getassetcounts) | **GET** /{customer}/{location}/{asset}/count | 
*DefaultApi* | [**getAssetDataTimerange**](Apis/DefaultApi.md#getassetdatatimerange) | **GET** /{customer}/{location}/{asset}/timeRange | 
*DefaultApi* | [**getAssetDatapoints**](Apis/DefaultApi.md#getassetdatapoints) | **GET** /{customer}/{location}/{asset} | 
*DefaultApi* | [**getAssetFactoryLocation**](Apis/DefaultApi.md#getassetfactorylocation) | **GET** /{customer}/{location}/{asset}/factoryLocations | 
*DefaultApi* | [**getAssetMaintenanceActivities**](Apis/DefaultApi.md#getassetmaintenanceactivities) | **GET** /{customer}/{location}/{asset}/maintenanceActivities | 
*DefaultApi* | [**getAssetMaintenanceComponents**](Apis/DefaultApi.md#getassetmaintenancecomponents) | **GET** /{customer}/{location}/{asset}/maintenanceComponents | 
*DefaultApi* | [**getAssetOee**](Apis/DefaultApi.md#getassetoee) | **GET** /{customer}/{location}/{asset}/oee | 
*DefaultApi* | [**getAssetOrderTable**](Apis/DefaultApi.md#getassetordertable) | **GET** /{customer}/{location}/{asset}/orderTable | 
*DefaultApi* | [**getAssetOrderTimeline**](Apis/DefaultApi.md#getassetordertimeline) | **GET** /{customer}/{location}/{asset}/orderTimeline | 
*DefaultApi* | [**getAssetPerformance**](Apis/DefaultApi.md#getassetperformance) | **GET** /{customer}/{location}/{asset}/performance | 
*DefaultApi* | [**getAssetProcessValue**](Apis/DefaultApi.md#getassetprocessvalue) | **GET** /{customer}/{location}/{asset}/process_{processValue} | 
*DefaultApi* | [**getAssetProductPicture**](Apis/DefaultApi.md#getassetproductpicture) | **GET** /{customer}/{location}/{asset}/productPicture | 
*DefaultApi* | [**getAssetProductionSpeed**](Apis/DefaultApi.md#getassetproductionspeed) | **GET** /{customer}/{location}/{asset}/productionSpeed | 
*DefaultApi* | [**getAssetQuality**](Apis/DefaultApi.md#getassetquality) | **GET** /{customer}/{location}/{asset}/quality | 
*DefaultApi* | [**getAssetQualityRate**](Apis/DefaultApi.md#getassetqualityrate) | **GET** /{customer}/{location}/{asset}/qualityRate | 
*DefaultApi* | [**getAssetShifts**](Apis/DefaultApi.md#getassetshifts) | **GET** /{customer}/{location}/{asset}/shifts | 
*DefaultApi* | [**getAssetStateHistogram**](Apis/DefaultApi.md#getassetstatehistogram) | **GET** /{customer}/{location}/{asset}/stateHistogram | 
*DefaultApi* | [**getAssetStates**](Apis/DefaultApi.md#getassetstates) | **GET** /{customer}/{location}/{asset}/state | 
*DefaultApi* | [**getAssetUmpcomingMaintenanceActivities**](Apis/DefaultApi.md#getassetumpcomingmaintenanceactivities) | **GET** /{customer}/{location}/{asset}/upcomingMaintenanceActivities | 
*DefaultApi* | [**getAssetUniqueImageId**](Apis/DefaultApi.md#getassetuniqueimageid) | **GET** /{customer}/{location}/{asset}/uniqueImageID | 
*DefaultApi* | [**getAssetUniqueProducts**](Apis/DefaultApi.md#getassetuniqueproducts) | **GET** /{customer}/{location}/{asset}/uniqueProducts | 
*DefaultApi* | [**getAssetUniqueProductsWithTags**](Apis/DefaultApi.md#getassetuniqueproductswithtags) | **GET** /{customer}/{location}/{asset}/uniqueProductWithTags | 
*DefaultApi* | [**getCurrentAssetRecommendation**](Apis/DefaultApi.md#getcurrentassetrecommendation) | **GET** /{customer}/{location}/{asset}/recommendation | 
*DefaultApi* | [**getCurrentAssetState**](Apis/DefaultApi.md#getcurrentassetstate) | **GET** /{customer}/{location}/{asset}/currentState | 
*DefaultApi* | [**getCustomerLocations**](Apis/DefaultApi.md#getcustomerlocations) | **GET** /{customer} | 
*DefaultApi* | [**getCustomerProductTags**](Apis/DefaultApi.md#getcustomerproducttags) | **GET** /{customer}/productTag | 
*DefaultApi* | [**getLocationAssets**](Apis/DefaultApi.md#getlocationassets) | **GET** /{customer}/{location} | 


<a name="documentation-for-models"></a>
## Documentation for Models

 - [AssetAggregatedStates](./Models/AssetAggregatedStates.md)
 - [AssetAvailability](./Models/AssetAvailability.md)
 - [AssetAverageChangeoverTime](./Models/AssetAverageChangeoverTime.md)
 - [AssetAverageCleaningTime](./Models/AssetAverageCleaningTime.md)
 - [AssetCounts](./Models/AssetCounts.md)
 - [AssetDataTimerange](./Models/AssetDataTimerange.md)
 - [AssetFactoryLocation](./Models/AssetFactoryLocation.md)
 - [AssetMaintenanceActivities](./Models/AssetMaintenanceActivities.md)
 - [AssetOee](./Models/AssetOee.md)
 - [AssetOrderTable](./Models/AssetOrderTable.md)
 - [AssetOrderTimeline](./Models/AssetOrderTimeline.md)
 - [AssetPerformance](./Models/AssetPerformance.md)
 - [AssetProcessValue](./Models/AssetProcessValue.md)
 - [AssetProductPicture](./Models/AssetProductPicture.md)
 - [AssetProductionSpeed](./Models/AssetProductionSpeed.md)
 - [AssetQuality](./Models/AssetQuality.md)
 - [AssetQualityRate](./Models/AssetQualityRate.md)
 - [AssetRecommendation](./Models/AssetRecommendation.md)
 - [AssetShifts](./Models/AssetShifts.md)
 - [AssetState](./Models/AssetState.md)
 - [AssetStateHistogram](./Models/AssetStateHistogram.md)
 - [AssetStates](./Models/AssetStates.md)
 - [AssetUmpcomingMaintenanceActivities](./Models/AssetUmpcomingMaintenanceActivities.md)
 - [AssetUniqueImageID](./Models/AssetUniqueImageID.md)
 - [AssetUniqueProducts](./Models/AssetUniqueProducts.md)
 - [AssetUniqueProductsWithTags](./Models/AssetUniqueProductsWithTags.md)
 - [CustomerProductTags](./Models/CustomerProductTags.md)


<a name="documentation-for-authorization"></a>
## Documentation for Authorization

<a name="BasicAuth"></a>
### BasicAuth

- **Type**: HTTP basic authentication

