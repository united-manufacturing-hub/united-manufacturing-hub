# DefaultApi

All URIs are relative to *https://api.industrial-analytics.net/factoryinput/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**publishJson**](DefaultApi.md#publishJson) | **POST** /{customer}/{location}/{asset}/{value} | Create MQTT Message from Rest call


<a name="publishJson"></a>
# **publishJson**
> publishJson(customer, location, asset, value, body)

Create MQTT Message from Rest call

    pulish mqtt message through rest

### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **customer** | **String**| name of the customer. only characters, numbers, - and _ allowed | [default to null]
 **location** | **String**| name of the location. only characters, numbers, - and _ allowed | [default to null]
 **asset** | **String**| name of the asset. only characters, numbers, - and _ allowed | [default to null]
 **value** | **String**| selected value. only characters, numbers, - and _ allowed | [default to null]
 **body** | **Object**| With the help of this Rest call you can create a MQTT message which is processed like a message that goes directly to the MQTT broker. The JSON can be any valid JSON from the MQTT Datamodell |

### Return type

null (empty response body)

### Authorization

[BasicAuth](../README.md#BasicAuth)

### HTTP request headers

- **Content-Type**: application/json
- **Accept**: Not defined

