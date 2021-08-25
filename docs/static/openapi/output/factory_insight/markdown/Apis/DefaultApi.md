# DefaultApi

All URIs are relative to *https://api.industrial-analytics.net/api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**getAssetAggregatedStates**](DefaultApi.md#getAssetAggregatedStates) | **GET** /{customer}/{location}/{asset}/aggregatedStates | 
[**getAssetAvailability**](DefaultApi.md#getAssetAvailability) | **GET** /{customer}/{location}/{asset}/availability | 
[**getAssetAverageChangeoverTime**](DefaultApi.md#getAssetAverageChangeoverTime) | **GET** /{customer}/{location}/{asset}/averageChangeoverTime | 
[**getAssetAverageCleaningTime**](DefaultApi.md#getAssetAverageCleaningTime) | **GET** /{customer}/{location}/{asset}/averageCleaningTime | 
[**getAssetCounts**](DefaultApi.md#getAssetCounts) | **GET** /{customer}/{location}/{asset}/count | 
[**getAssetDataTimerange**](DefaultApi.md#getAssetDataTimerange) | **GET** /{customer}/{location}/{asset}/timeRange | 
[**getAssetDatapoints**](DefaultApi.md#getAssetDatapoints) | **GET** /{customer}/{location}/{asset} | 
[**getAssetFactoryLocation**](DefaultApi.md#getAssetFactoryLocation) | **GET** /{customer}/{location}/{asset}/factoryLocations | 
[**getAssetMaintenanceActivities**](DefaultApi.md#getAssetMaintenanceActivities) | **GET** /{customer}/{location}/{asset}/maintenanceActivities | 
[**getAssetMaintenanceComponents**](DefaultApi.md#getAssetMaintenanceComponents) | **GET** /{customer}/{location}/{asset}/maintenanceComponents | 
[**getAssetOee**](DefaultApi.md#getAssetOee) | **GET** /{customer}/{location}/{asset}/oee | 
[**getAssetOrderTable**](DefaultApi.md#getAssetOrderTable) | **GET** /{customer}/{location}/{asset}/orderTable | 
[**getAssetOrderTimeline**](DefaultApi.md#getAssetOrderTimeline) | **GET** /{customer}/{location}/{asset}/orderTimeline | 
[**getAssetPerformance**](DefaultApi.md#getAssetPerformance) | **GET** /{customer}/{location}/{asset}/performance | 
[**getAssetProcessValue**](DefaultApi.md#getAssetProcessValue) | **GET** /{customer}/{location}/{asset}/process_{processValue} | 
[**getAssetProductPicture**](DefaultApi.md#getAssetProductPicture) | **GET** /{customer}/{location}/{asset}/productPicture | 
[**getAssetProductionSpeed**](DefaultApi.md#getAssetProductionSpeed) | **GET** /{customer}/{location}/{asset}/productionSpeed | 
[**getAssetQuality**](DefaultApi.md#getAssetQuality) | **GET** /{customer}/{location}/{asset}/quality | 
[**getAssetQualityRate**](DefaultApi.md#getAssetQualityRate) | **GET** /{customer}/{location}/{asset}/qualityRate | 
[**getAssetShifts**](DefaultApi.md#getAssetShifts) | **GET** /{customer}/{location}/{asset}/shifts | 
[**getAssetStateHistogram**](DefaultApi.md#getAssetStateHistogram) | **GET** /{customer}/{location}/{asset}/stateHistogram | 
[**getAssetStates**](DefaultApi.md#getAssetStates) | **GET** /{customer}/{location}/{asset}/state | 
[**getAssetUmpcomingMaintenanceActivities**](DefaultApi.md#getAssetUmpcomingMaintenanceActivities) | **GET** /{customer}/{location}/{asset}/upcomingMaintenanceActivities | 
[**getAssetUniqueImageId**](DefaultApi.md#getAssetUniqueImageId) | **GET** /{customer}/{location}/{asset}/uniqueImageID | 
[**getAssetUniqueProducts**](DefaultApi.md#getAssetUniqueProducts) | **GET** /{customer}/{location}/{asset}/uniqueProducts | 
[**getAssetUniqueProductsWithTags**](DefaultApi.md#getAssetUniqueProductsWithTags) | **GET** /{customer}/{location}/{asset}/uniqueProductWithTags | 
[**getCurrentAssetRecommendation**](DefaultApi.md#getCurrentAssetRecommendation) | **GET** /{customer}/{location}/{asset}/recommendation | 
[**getCurrentAssetState**](DefaultApi.md#getCurrentAssetState) | **GET** /{customer}/{location}/{asset}/currentState | 
[**getCustomerLocations**](DefaultApi.md#getCustomerLocations) | **GET** /{customer} | 
[**getCustomerProductTags**](DefaultApi.md#getCustomerProductTags) | **GET** /{customer}/productTag | 
[**getLocationAssets**](DefaultApi.md#getLocationAssets) | **GET** /{customer}/{location} | 


<a name="getAssetAggregatedStates"></a>
# **getAssetAggregatedStates**
> asset_aggregated_states getAssetAggregatedStates(customer, location, asset, from, to, includeRunning, aggregationType, keepStatesInteger)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]
 **includeRunning** | **Boolean**| If set to true the returned data will include running states as well. This makes sense for a pie chart showing the machine states, but for a pareto bar chart you would set this to false. | [default to null]
 **aggregationType** | **Integer**| With this parameter you can specify how the data should be aggregated. 0 means aggregating it over the entire time range. 1 means aggregating it by hours in a day. | [default to null]
 **keepStatesInteger** | **Boolean**| If set to true all returned states will be numbers at not strings | [optional] [default to false]

### Return type

[**asset_aggregated_states**](../Models/asset_aggregated_states.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetAvailability"></a>
# **getAssetAvailability**
> asset_availability getAssetAvailability(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**asset_availability**](../Models/asset_availability.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetAverageChangeoverTime"></a>
# **getAssetAverageChangeoverTime**
> asset_average_changeover_time getAssetAverageChangeoverTime(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**asset_average_changeover_time**](../Models/asset_average_changeover_time.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetAverageCleaningTime"></a>
# **getAssetAverageCleaningTime**
> asset_average_cleaning_time getAssetAverageCleaningTime(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**asset_average_cleaning_time**](../Models/asset_average_cleaning_time.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetCounts"></a>
# **getAssetCounts**
> asset_counts getAssetCounts(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**asset_counts**](../Models/asset_counts.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetDataTimerange"></a>
# **getAssetDataTimerange**
> asset_data_timerange getAssetDataTimerange(customer, location, asset)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]

### Return type

[**asset_data_timerange**](../Models/asset_data_timerange.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetDatapoints"></a>
# **getAssetDatapoints**
> List getAssetDatapoints(customer, location, asset)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]

### Return type

[**List**](../Models/string.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetFactoryLocation"></a>
# **getAssetFactoryLocation**
> asset_factory_location getAssetFactoryLocation(customer, location, asset)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]

### Return type

[**asset_factory_location**](../Models/asset_factory_location.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetMaintenanceActivities"></a>
# **getAssetMaintenanceActivities**
> asset_maintenance_activities getAssetMaintenanceActivities(customer, location, asset)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]

### Return type

[**asset_maintenance_activities**](../Models/asset_maintenance_activities.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetMaintenanceComponents"></a>
# **getAssetMaintenanceComponents**
> List getAssetMaintenanceComponents(customer, location, asset)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]

### Return type

[**List**](../Models/string.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetOee"></a>
# **getAssetOee**
> asset_oee getAssetOee(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**asset_oee**](../Models/asset_oee.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetOrderTable"></a>
# **getAssetOrderTable**
> asset_order_table getAssetOrderTable(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**asset_order_table**](../Models/asset_order_table.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetOrderTimeline"></a>
# **getAssetOrderTimeline**
> asset_order_timeline getAssetOrderTimeline(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**asset_order_timeline**](../Models/asset_order_timeline.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetPerformance"></a>
# **getAssetPerformance**
> asset_performance getAssetPerformance(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**asset_performance**](../Models/asset_performance.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetProcessValue"></a>
# **getAssetProcessValue**
> asset_process_value getAssetProcessValue(customer, location, asset, from, to, processValue)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]
 **processValue** | **String**| name of the process value to retrieve data | [default to null]

### Return type

[**asset_process_value**](../Models/asset_process_value.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetProductPicture"></a>
# **getAssetProductPicture**
> asset_product_picture getAssetProductPicture(uniqueImageID, customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **uniqueImageID** | **String**|  | [default to null]
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**asset_product_picture**](../Models/asset_product_picture.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetProductionSpeed"></a>
# **getAssetProductionSpeed**
> asset_production_speed getAssetProductionSpeed(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**asset_production_speed**](../Models/asset_production_speed.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetQuality"></a>
# **getAssetQuality**
> asset_quality getAssetQuality(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**asset_quality**](../Models/asset_quality.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetQualityRate"></a>
# **getAssetQualityRate**
> asset_quality_rate getAssetQualityRate(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**asset_quality_rate**](../Models/asset_quality_rate.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetShifts"></a>
# **getAssetShifts**
> asset_shifts getAssetShifts(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**asset_shifts**](../Models/asset_shifts.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetStateHistogram"></a>
# **getAssetStateHistogram**
> asset_state_histogram getAssetStateHistogram(customer, location, asset, from, to, includeRunning, keepStatesInteger)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]
 **includeRunning** | **Boolean**| If set to true the returned data will include running states as well. This makes sense for a pie chart showing the machine states, but for a pareto bar chart you would set this to false. | [default to null]
 **keepStatesInteger** | **Boolean**| If set to true all returned states will be numbers at not strings | [optional] [default to false]

### Return type

[**asset_state_histogram**](../Models/asset_state_histogram.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetStates"></a>
# **getAssetStates**
> asset_states getAssetStates(customer, location, asset, from, to, keepStatesInteger)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]
 **keepStatesInteger** | **Boolean**| If set to true all returned states will be numbers at not strings | [optional] [default to false]

### Return type

[**asset_states**](../Models/asset_states.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetUmpcomingMaintenanceActivities"></a>
# **getAssetUmpcomingMaintenanceActivities**
> asset_umpcoming_maintenance_activities getAssetUmpcomingMaintenanceActivities(customer, location, asset)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]

### Return type

[**asset_umpcoming_maintenance_activities**](../Models/asset_umpcoming_maintenance_activities.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetUniqueImageId"></a>
# **getAssetUniqueImageId**
> asset_unique_ImageID getAssetUniqueImageId(customer, location, asset, from, to, NumberOfImages, TagName, TagValue)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]
 **NumberOfImages** | **Integer**| max number of images to get, ordered by time desc, this means 1 yields the last image recorded, this does not overwrite, but specify the number of images to get, aka the number of images returned can only get lower | [optional] [default to null]
 **TagName** | **String**| filters output by entries with a tag of type Tag Type | [optional] [default to null]
 **TagValue** | **String**| filters output by entries with tag of type Tag Type and this vlaue. requires tag name to be set | [optional] [default to null]

### Return type

[**asset_unique_ImageID**](../Models/asset_unique_ImageID.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetUniqueProducts"></a>
# **getAssetUniqueProducts**
> asset_unique_products getAssetUniqueProducts(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**asset_unique_products**](../Models/asset_unique_products.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetUniqueProductsWithTags"></a>
# **getAssetUniqueProductsWithTags**
> asset_unique_products_with_tags getAssetUniqueProductsWithTags(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**asset_unique_products_with_tags**](../Models/asset_unique_products_with_tags.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getCurrentAssetRecommendation"></a>
# **getCurrentAssetRecommendation**
> asset_recommendation getCurrentAssetRecommendation(customer, location, asset)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]

### Return type

[**asset_recommendation**](../Models/asset_recommendation.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getCurrentAssetState"></a>
# **getCurrentAssetState**
> asset_state getCurrentAssetState(customer, location, asset)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]

### Return type

[**asset_state**](../Models/asset_state.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getCustomerLocations"></a>
# **getCustomerLocations**
> List getCustomerLocations(customer)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]

### Return type

[**List**](../Models/string.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getCustomerProductTags"></a>
# **getCustomerProductTags**
> customer_product_tags getCustomerProductTags(customer, uniqueProductID)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **uniqueProductID** | **String**| unique id of the product to search for | [default to null]

### Return type

[**customer_product_tags**](../Models/customer_product_tags.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getLocationAssets"></a>
# **getLocationAssets**
> List getLocationAssets(customer, location)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]

### Return type

[**List**](../Models/string.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

