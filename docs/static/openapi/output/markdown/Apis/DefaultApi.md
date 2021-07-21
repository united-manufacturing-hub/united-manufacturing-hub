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
[**getAssetProductionSpeed**](DefaultApi.md#getAssetProductionSpeed) | **GET** /{customer}/{location}/{asset}/productionSpeed | 
[**getAssetQuality**](DefaultApi.md#getAssetQuality) | **GET** /{customer}/{location}/{asset}/quality | 
[**getAssetQualityRate**](DefaultApi.md#getAssetQualityRate) | **GET** /{customer}/{location}/{asset}/qualityRate | 
[**getAssetShifts**](DefaultApi.md#getAssetShifts) | **GET** /{customer}/{location}/{asset}/shifts | 
[**getAssetStateHistogram**](DefaultApi.md#getAssetStateHistogram) | **GET** /{customer}/{location}/{asset}/stateHistogram | 
[**getAssetStates**](DefaultApi.md#getAssetStates) | **GET** /{customer}/{location}/{asset}/state | 
[**getAssetUmpcomingMaintenanceActivities**](DefaultApi.md#getAssetUmpcomingMaintenanceActivities) | **GET** /{customer}/{location}/{asset}/upcomingMaintenanceActivities | 
[**getAssetUniqueProducts**](DefaultApi.md#getAssetUniqueProducts) | **GET** /{customer}/{location}/{asset}/uniqueProducts | 
[**getCurrentAssetRecommendation**](DefaultApi.md#getCurrentAssetRecommendation) | **GET** /{customer}/{location}/{asset}/recommendation | 
[**getCurrentAssetState**](DefaultApi.md#getCurrentAssetState) | **GET** /{customer}/{location}/{asset}/currentState | 
[**getCustomerLocations**](DefaultApi.md#getCustomerLocations) | **GET** /{customer} | 
[**getLocationAssets**](DefaultApi.md#getLocationAssets) | **GET** /{customer}/{location} | 


<a name="getAssetAggregatedStates"></a>
# **getAssetAggregatedStates**
> inline_response_200_5 getAssetAggregatedStates(customer, location, asset, from, to, includeRunning, aggregationType, keepStatesInteger)



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

[**inline_response_200_5**](../Models/inline_response_200_5.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetAvailability"></a>
# **getAssetAvailability**
> inline_response_200_6 getAssetAvailability(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**inline_response_200_6**](../Models/inline_response_200_6.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetAverageChangeoverTime"></a>
# **getAssetAverageChangeoverTime**
> inline_response_200_16 getAssetAverageChangeoverTime(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**inline_response_200_16**](../Models/inline_response_200_16.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetAverageCleaningTime"></a>
# **getAssetAverageCleaningTime**
> inline_response_200_15 getAssetAverageCleaningTime(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**inline_response_200_15**](../Models/inline_response_200_15.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetCounts"></a>
# **getAssetCounts**
> inline_response_200_1 getAssetCounts(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**inline_response_200_1**](../Models/inline_response_200_1.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetDataTimerange"></a>
# **getAssetDataTimerange**
> inline_response_200_4 getAssetDataTimerange(customer, location, asset)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]

### Return type

[**inline_response_200_4**](../Models/inline_response_200_4.md)

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
> inline_response_200_14 getAssetFactoryLocation(customer, location, asset)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]

### Return type

[**inline_response_200_14**](../Models/inline_response_200_14.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetMaintenanceActivities"></a>
# **getAssetMaintenanceActivities**
> inline_response_200_18 getAssetMaintenanceActivities(customer, location, asset)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]

### Return type

[**inline_response_200_18**](../Models/inline_response_200_18.md)

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
> inline_response_200_9 getAssetOee(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**inline_response_200_9**](../Models/inline_response_200_9.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetOrderTable"></a>
# **getAssetOrderTable**
> inline_response_200_20 getAssetOrderTable(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**inline_response_200_20**](../Models/inline_response_200_20.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetOrderTimeline"></a>
# **getAssetOrderTimeline**
> inline_response_200_21 getAssetOrderTimeline(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**inline_response_200_21**](../Models/inline_response_200_21.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetPerformance"></a>
# **getAssetPerformance**
> inline_response_200_7 getAssetPerformance(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**inline_response_200_7**](../Models/inline_response_200_7.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetProcessValue"></a>
# **getAssetProcessValue**
> inline_response_200_22 getAssetProcessValue(customer, location, asset, from, to, processValue)



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

[**inline_response_200_22**](../Models/inline_response_200_22.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetProductionSpeed"></a>
# **getAssetProductionSpeed**
> inline_response_200_11 getAssetProductionSpeed(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**inline_response_200_11**](../Models/inline_response_200_11.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetQuality"></a>
# **getAssetQuality**
> inline_response_200_8 getAssetQuality(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**inline_response_200_8**](../Models/inline_response_200_8.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetQualityRate"></a>
# **getAssetQualityRate**
> inline_response_200_12 getAssetQualityRate(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**inline_response_200_12**](../Models/inline_response_200_12.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetShifts"></a>
# **getAssetShifts**
> inline_response_200_10 getAssetShifts(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**inline_response_200_10**](../Models/inline_response_200_10.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetStateHistogram"></a>
# **getAssetStateHistogram**
> inline_response_200_13 getAssetStateHistogram(customer, location, asset, from, to, includeRunning, keepStatesInteger)



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

[**inline_response_200_13**](../Models/inline_response_200_13.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetStates"></a>
# **getAssetStates**
> inline_response_200 getAssetStates(customer, location, asset, from, to, keepStatesInteger)



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

[**inline_response_200**](../Models/inline_response_200.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetUmpcomingMaintenanceActivities"></a>
# **getAssetUmpcomingMaintenanceActivities**
> inline_response_200_17 getAssetUmpcomingMaintenanceActivities(customer, location, asset)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]

### Return type

[**inline_response_200_17**](../Models/inline_response_200_17.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getAssetUniqueProducts"></a>
# **getAssetUniqueProducts**
> inline_response_200_19 getAssetUniqueProducts(customer, location, asset, from, to)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]
 **from** | **date**| Get data from this timestamp on (in RFC3339 format) | [default to null]
 **to** | **date**| Get data till this timestamp (in RFC3339 format) | [default to null]

### Return type

[**inline_response_200_19**](../Models/inline_response_200_19.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getCurrentAssetRecommendation"></a>
# **getCurrentAssetRecommendation**
> inline_response_200_3 getCurrentAssetRecommendation(customer, location, asset)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]

### Return type

[**inline_response_200_3**](../Models/inline_response_200_3.md)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

<a name="getCurrentAssetState"></a>
# **getCurrentAssetState**
> inline_response_200_2 getCurrentAssetState(customer, location, asset)



### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer | [default to null]
 **location** | **String**| name of the location | [default to null]
 **asset** | **String**| name of the asset | [default to null]

### Return type

[**inline_response_200_2**](../Models/inline_response_200_2.md)

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

