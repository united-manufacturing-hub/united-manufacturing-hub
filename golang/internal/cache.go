// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	jsoniter "github.com/json-iterator/go"
	"github.com/patrickmn/go-cache"
	_ "github.com/patrickmn/go-cache"
	"github.com/rung/go-safecast"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
	"strconv"
	"time"
)

var rdb *redis.Client
var ctx = context.Background()
var memCache *cache.Cache

var redisDataExpiration time.Duration
var memoryDataExpiration time.Duration

const NullStr = "null"

// InitCache initializes a redis cache
func InitCache(redisURI string, redisPassword string, redisDB int, dryRun bool) {

	if dryRun {
		zap.S().Infof("Running cache in DRY_RUN mode. This means that cache will not be used") // "... and it stays nil"
		return
	}
	rdb = redis.NewClient(&redis.Options{
		Addr:     redisURI,
		Password: redisPassword, // no password set
		DB:       redisDB,       // use default DB
	})

	redisDataExpiration = 12 * time.Hour
	memoryDataExpiration = 10 * time.Second

	memCache = cache.New(memoryDataExpiration, 20*time.Second)
}

func InitCacheWithoutRedis() {
	memoryDataExpiration = 10 * time.Second
	memCache = cache.New(memoryDataExpiration, 20*time.Second)
}

// https://github.com/united-manufacturing-hub/structHashCmp

var json = jsoniter.ConfigCompatibleWithStandardLibrary

// AsHash returns a hash for a given interface
// Note: this is not a cryptographic hash, but a hash for comparison purposes
// Also note: do not use this with structs that contain channels or functions, it will panic
func AsHash(o interface{}) string {
	marshal, err := json.Marshal(o)
	if err != nil {
		zap.S().Fatalf("Failed to marshal object: %v", err)
	}

	digester := xxh3.New()
	_, err = digester.Write(marshal)
	if err != nil {
		// err is always nil
		zap.S().Fatalf("Failed to write to digester: %v", err)
	}
	return fmt.Sprintf("%x", digester.Sum(nil))
}

// GetProcessStatesFromCache gets process states from cache
func GetProcessStatesFromCache(key string) (processedStateArray []datamodel.StateEntry, cacheHit bool) {
	if rdb == nil { // only the case during tests
		////zap.S().Errorf("rdb == nil")
		return
	}

	value, err := rdb.Get(ctx, key).Result()

	if errors.Is(err, redis.Nil) { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == NullStr {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		// https://itnext.io/storing-go-structs-in-redis-using-rejson-dab7f8fc0053
		b := []byte(value)
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		err = json.Unmarshal(b, &processedStateArray)
		if err != nil {
			zap.S().Errorf("json Unmarshal", b)
			return
		}

		cacheHit = true
	}
	return
}

// StoreProcessStatesToCache stores process states to the cache
func StoreProcessStatesToCache(key string, processedStateArray []datamodel.StateEntry) {
	if rdb == nil { // only the case during tests
		////zap.S().Errorf("rdb == nil")
		return
	}

	if processedStateArray == nil {
		// zap.S().Debugf("input is empty. aborting storing into database.")
		return
	}

	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	b, err := json.Marshal(&processedStateArray)
	if err != nil {
		zap.S().Errorf("json marshall")
		return
	}

	err = RedisSafeSet(ctx, key, b, redisDataExpiration)
	if err != nil {
		zap.S().Errorf("redis failed: %#v", err)
		return
	}
}

// GetCalculatateLowSpeedStatesFromCache get low speed states from cache
func GetCalculatateLowSpeedStatesFromCache(
	from time.Time,
	to time.Time,
	assetID uint32) (processedStateArray []datamodel.StateEntry, cacheHit bool) {
	if rdb == nil { // only the case during tests
		////zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("CalculatateLowSpeedStates-%s-%s-%d", from, to, assetID)

	value, err := rdb.Get(ctx, key).Result()

	if errors.Is(err, redis.Nil) { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == NullStr {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		// https://itnext.io/storing-go-structs-in-redis-using-rejson-dab7f8fc0053
		b := []byte(value)
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		err = json.Unmarshal(b, &processedStateArray)
		if err != nil {
			zap.S().Errorf("json Unmarshal", b)
			return
		}

		cacheHit = true
	}

	return
}

// StoreCalculatateLowSpeedStatesToCache stores low speed states to cache
func StoreCalculatateLowSpeedStatesToCache(
	from time.Time,
	to time.Time,
	assetID uint32,
	processedStateArray []datamodel.StateEntry) {
	if rdb == nil { // only the case during tests
		////zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("CalculatateLowSpeedStates-%s-%s-%d", from, to, assetID)

	if processedStateArray == nil {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	b, err := json.Marshal(&processedStateArray)
	if err != nil {
		zap.S().Errorf("json marshall: %+v", err)
		return
	}

	err = RedisSafeSet(ctx, key, b, redisDataExpiration)
	if err != nil {
		zap.S().Errorf("redis failed: %+v", err)
		return
	}
}

// GetStatesRawFromCache gets raw states from cache
func GetStatesRawFromCache(
	assetID uint32,
	from time.Time,
	to time.Time,
	configuration datamodel.CustomerConfiguration) (data []datamodel.StateEntry, cacheHit bool) {
	if rdb == nil { // only the case during tests
		////zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getStatesRawFromCache-%d-%s-%s-%s", assetID, from, to, AsHash(configuration))

	value, err := rdb.Get(ctx, key).Result()

	if errors.Is(err, redis.Nil) { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == NullStr {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		// https://itnext.io/storing-go-structs-in-redis-using-rejson-dab7f8fc0053
		b := []byte(value)
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		err = json.Unmarshal(b, &data)
		if err != nil {
			zap.S().Errorf("json Unmarshal", b)
			return
		}

		cacheHit = true
	}

	return
}

// StoreRawStatesToCache stores raw states to cache
func StoreRawStatesToCache(
	assetID uint32,
	from time.Time,
	to time.Time,
	configuration datamodel.CustomerConfiguration,
	data []datamodel.StateEntry) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getStatesRawFromCache-%d-%s-%s-%s", assetID, from, to, AsHash(configuration))

	if data == nil {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	b, err := json.Marshal(&data)
	if err != nil {
		zap.S().Errorf("json marshall")
		return
	}

	err = RedisSafeSet(ctx, key, b, redisDataExpiration)

	if err != nil {
		zap.S().Errorf("redis failed: %#v", err)
		return
	}
}

// GetRawShiftsFromCache gets raw shifts from cache
func GetRawShiftsFromCache(
	assetID uint32,
	from time.Time,
	to time.Time,
	configuration datamodel.CustomerConfiguration) (data []datamodel.ShiftEntry, cacheHit bool) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getRawShiftsFromCache-%d-%s-%s-%s", assetID, from, to, AsHash(configuration))

	value, err := rdb.Get(ctx, key).Result()

	if errors.Is(err, redis.Nil) { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == NullStr {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		// https://itnext.io/storing-go-structs-in-redis-using-rejson-dab7f8fc0053
		b := []byte(value)
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		err = json.Unmarshal(b, &data)
		if err != nil {
			zap.S().Errorf("json Unmarshal", b)
			return
		}

		cacheHit = true
	}

	return
}

// StoreRawShiftsToCache stores raw shifts to cache
func StoreRawShiftsToCache(
	assetID uint32,
	from time.Time,
	to time.Time,
	configuration datamodel.CustomerConfiguration,
	data []datamodel.ShiftEntry) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getRawShiftsFromCache-%d-%s-%s-%s", assetID, from, to, AsHash(configuration))

	if data == nil {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	b, err := json.Marshal(&data)
	if err != nil {
		zap.S().Errorf("json marshall")
		return
	}

	err = RedisSafeSet(ctx, key, b, redisDataExpiration)
	if err != nil {
		zap.S().Errorf("redis failed: %#v", err)
		return
	}
}

// GetRawCountsFromCache gets raw counts from cache
func GetRawCountsFromCache(assetID uint32, from time.Time, to time.Time) (data []datamodel.CountEntry, cacheHit bool) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getRawCountsFromCache-%d-%s-%s", assetID, from, to)
	value, err := rdb.Get(ctx, key).Result()

	if errors.Is(err, redis.Nil) { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == NullStr {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		// https://itnext.io/storing-go-structs-in-redis-using-rejson-dab7f8fc0053
		b := []byte(value)
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		err = json.Unmarshal(b, &data)
		if err != nil {
			zap.S().Errorf("json Unmarshal", b)
			return
		}

		cacheHit = true
	}

	return
}

// StoreRawCountsToCache stores raw counts to cache
func StoreRawCountsToCache(assetID uint32, from time.Time, to time.Time, data []datamodel.CountEntry) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getRawCountsFromCache-%d-%s-%s", assetID, from, to)

	if data == nil {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	b, err := json.Marshal(&data)
	if err != nil {
		zap.S().Errorf("json marshall")
		return
	}

	err = RedisSafeSet(ctx, key, b, redisDataExpiration)
	if err != nil {
		zap.S().Errorf("redis failed: %#v", err)
		return
	}
}

// GetAverageStateTimeFromCache gets average state time from cache
func GetAverageStateTimeFromCache(key string) (data []interface{}, cacheHit bool) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	value, err := rdb.Get(ctx, key).Result()

	if errors.Is(err, redis.Nil) { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == NullStr {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		// https://itnext.io/storing-go-structs-in-redis-using-rejson-dab7f8fc0053
		b := []byte(value)
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		err = json.Unmarshal(b, &data)
		if err != nil {
			zap.S().Errorf("json Unmarshal", b)
			return
		}

		cacheHit = true
	}

	return
}

// StoreAverageStateTimeToCache stores average state time to cache
func StoreAverageStateTimeToCache(key string, data []interface{}) {

	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	if data == nil {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	b, err := json.Marshal(&data)
	if err != nil {
		zap.S().Errorf("json marshall")
		return
	}

	err = RedisSafeSet(ctx, key, b, redisDataExpiration)
	if err != nil {
		zap.S().Errorf("redis failed: %#v", err)
		return
	}
}

// GetDistinctProcessValuesFromCache gets distinct process values from cache
func GetDistinctProcessValuesFromCache(customerID string, location string, assetID string) (
	data []string,
	cacheHit bool) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getDistinctProcessValues-%s-%s-%s", customerID, location, assetID)

	value, err := rdb.Get(ctx, key).Result()

	if errors.Is(err, redis.Nil) { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == NullStr {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		// https://itnext.io/storing-go-structs-in-redis-using-rejson-dab7f8fc0053
		b := []byte(value)
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		err = json.Unmarshal(b, &data)
		if err != nil {
			zap.S().Errorf("json Unmarshal", b)
			return
		}

		cacheHit = true
	}

	return
}

// GetDistinctProcessValuesStringFromCache gets distinct process values from cache
func GetDistinctProcessValuesStringFromCache(customerID string, location string, assetID string) (
	data []string,
	cacheHit bool) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getDistinctProcessValuesString-%s-%s-%s", customerID, location, assetID)

	value, err := rdb.Get(ctx, key).Result()

	if errors.Is(err, redis.Nil) { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == NullStr {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		// https://itnext.io/storing-go-structs-in-redis-using-rejson-dab7f8fc0053
		b := []byte(value)
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		err = json.Unmarshal(b, &data)
		if err != nil {
			zap.S().Errorf("json Unmarshal", b)
			return
		}

		cacheHit = true
	}

	return
}

// StoreDistinctProcessValuesStringToCache stores distinct process values to cache
func StoreDistinctProcessValuesStringToCache(customerID string, location string, assetID string, data []string) {

	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getDistinctProcessValuesString-%s-%s-%s", customerID, location, assetID)

	if data == nil {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	b, err := json.Marshal(&data)
	if err != nil {
		zap.S().Errorf("json marshall")
		return
	}

	err = RedisSafeSet(ctx, key, b, 5*time.Minute)
	if err != nil {
		zap.S().Errorf("redis failed: %#v", err)
		return
	}
}

func StoreCustomerConfigurationToCache(customerID string, data datamodel.CustomerConfiguration) {

	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("GetCustomerConfigurationFromCache-%s", customerID)

	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	b, err := json.Marshal(&data)
	if err != nil {
		zap.S().Errorf("json marshall")
		return
	}

	err = RedisSafeSet(ctx, key, b, redisDataExpiration)
	if err != nil {
		zap.S().Errorf("redis failed: %#v", err)
		return
	}
}

// StoreDistinctProcessValuesToCache stores distinct process values to cache
func StoreDistinctProcessValuesToCache(customerID string, location string, assetID string, data []string) {

	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getDistinctProcessValues-%s-%s-%s", customerID, location, assetID)

	if data == nil {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	b, err := json.Marshal(&data)
	if err != nil {
		zap.S().Errorf("json marshall")
		return
	}

	err = RedisSafeSet(ctx, key, b, 5*time.Minute)
	if err != nil {
		zap.S().Errorf("redis failed: %#v", err)
		return
	}
}

// GetCustomerConfigurationFromCache gets customer configuration from cache
func GetCustomerConfigurationFromCache(customerID string) (data datamodel.CustomerConfiguration, cacheHit bool) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("GetCustomerConfigurationFromCache-%s", customerID)

	value, err := rdb.Get(ctx, key).Result()

	if errors.Is(err, redis.Nil) { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == NullStr {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		// https://itnext.io/storing-go-structs-in-redis-using-rejson-dab7f8fc0053
		b := []byte(value)
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		err = json.Unmarshal(b, &data)
		if err != nil {
			zap.S().Errorf("json Unmarshal", b)
			return
		}

		cacheHit = true
	}

	return
}

// GetAssetIDFromCache gets asset id from cache
func GetAssetIDFromCache(customerID string, location string, assetID string) (DBassetID uint32, cacheHit bool) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getAssetID-%s-%s-%s", customerID, location, assetID)

	value, err := rdb.Get(ctx, key).Result()

	if errors.Is(err, redis.Nil) { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == NullStr {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		var RawDBassetID int
		RawDBassetID, err = strconv.Atoi(value)
		if err != nil {
			zap.S().Errorf("error converting value to integer", key, err)
			return
		}

		var IDBassetID int32
		IDBassetID, err = safecast.Int32(RawDBassetID)
		if err != nil {
			zap.S().Errorf("error converting value to integer", key, err)
			return
		}

		DBassetID = uint32(IDBassetID)

		cacheHit = true
	}
	return
}

// StoreAssetIDToCache stores asset id to cache
func StoreAssetIDToCache(customerID string, location string, assetID string, DBassetID uint32) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getAssetID-%s-%s-%s", customerID, location, assetID)

	if DBassetID == 0 {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	b := strconv.Itoa(int(DBassetID))

	err := RedisSafeSet(ctx, key, []byte(b), redisDataExpiration)
	if err != nil {
		zap.S().Errorf("redis failed: %#v", err)
		return
	}
}

// GetUniqueProductIDFromCache gets uniqueProduct from cache
func GetUniqueProductIDFromCache(aid string, DBassetID uint32) (uid uint32, cacheHit bool) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getUniqueProductID-%s-%d", aid, DBassetID)

	value, err := rdb.Get(ctx, key).Result()

	if errors.Is(err, redis.Nil) { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == NullStr {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		var RawUID int
		RawUID, err = strconv.Atoi(value)
		if err != nil {
			zap.S().Errorf("error converting value to integer", key, err)
			return
		}

		var iuid int32
		iuid, err = safecast.Int32(RawUID)
		if err != nil {
			zap.S().Errorf("error converting value to integer", key, err)
			return
		}

		uid = uint32(iuid)

		cacheHit = true
	}
	return
}

// StoreUniqueProductIDToCache stores uniqueProductID to cache
func StoreUniqueProductIDToCache(aid string, DBassetID uint32, uid uint32) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getUniqueProductID-%s-%d", aid, DBassetID)

	if uid == 0 {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	b := strconv.Itoa(int(uid))

	err := RedisSafeSet(ctx, key, []byte(b), redisDataExpiration)
	if err != nil {
		zap.S().Errorf("redis failed: %#v", err)
		return
	}
}

// GetProductIDFromCache gets Product from cache
func GetProductIDFromCache(productName int32, DBassetID uint32) (DBProductId uint32, cacheHit bool) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getProductID-%d-%d", productName, DBassetID)

	value, err := rdb.Get(ctx, key).Result()

	if errors.Is(err, redis.Nil) { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == NullStr {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		var RawUID int
		RawUID, err = strconv.Atoi(value)
		if err != nil {
			zap.S().Errorf("error converting value to integer", key, err)
			return
		}

		var iuid int32
		iuid, err = safecast.Int32(RawUID)
		if err != nil {
			zap.S().Errorf("error converting value to integer", key, err)
			return
		}

		DBProductId = uint32(iuid)

		cacheHit = true
	}
	return
}

// StoreUniqueProductIDToCache stores uniqueProductID to cache
func StoreProductIDToCache(productName int32, DBassetID uint32, DBProductId uint32) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getProductID-%d-%d", productName, DBassetID)

	if DBProductId == 0 {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	b := strconv.Itoa(int(DBProductId))

	err := RedisSafeSet(ctx, key, []byte(b), redisDataExpiration)
	if err != nil {
		zap.S().Errorf("redis failed: %#v", err)
		return
	}
}

// StoreEnterpriseIDToCache stores enterprise id to cache
func StoreEnterpriseIDToCache(enterpriseName string, enterpriseID uint32) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getEnterpriseName-%s", enterpriseName)

	if enterpriseID == 0 {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	b := strconv.Itoa(int(enterpriseID))

	err := RedisSafeSet(ctx, key, []byte(b), redisDataExpiration)
	if err != nil {
		zap.S().Errorf("redis failed: %#v", err)
		return
	}
}

// GetEnterpriseIDFromCache gets enterprise id from cache
func GetEnterpriseIDFromCache(enterpriseName string) (enterpriseId uint32, cacheHit bool) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getEnterpriseName-%s", enterpriseName)

	value, err := rdb.Get(ctx, key).Result()

	if errors.Is(err, redis.Nil) { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("[GetEnterpriseIDFromCache] error getting key from redis")
		return
	} else if value == NullStr {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		var rawEnterpriseId int
		rawEnterpriseId, err = strconv.Atoi(value)
		if err != nil {
			zap.S().Errorf("[GetEnterpriseIDFromCache] error converting value to integer")
			return
		}

		var int32EnterpriseId int32
		int32EnterpriseId, err = safecast.Int32(rawEnterpriseId)
		if err != nil {
			zap.S().Errorf("[GetEnterpriseIDFromCache] error converting value to integer")
			return
		}

		enterpriseId = uint32(int32EnterpriseId)

		cacheHit = true
	}
	return
}

// StoreSiteIDToCache stores site id to cache
func StoreSiteIDToCache(enterpriseId uint32, siteName string, siteID uint32) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	strEnterpriseId := strconv.Itoa(int(enterpriseId))

	key := fmt.Sprintf("getSiteName-%s-%s", strEnterpriseId, siteName)

	if siteID == 0 {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	b := strconv.Itoa(int(siteID))

	err := RedisSafeSet(ctx, key, []byte(b), redisDataExpiration)
	if err != nil {
		zap.S().Errorf("redis failed: %#v", err)
		return
	}
}

// GetSiteIDFromCache gets site id from cache
func GetSiteIDFromCache(enterpriseId uint32, siteName string) (siteId uint32, cacheHit bool) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	strEnterpriseId := strconv.Itoa(int(enterpriseId))

	key := fmt.Sprintf("getSiteName-%s-%s", strEnterpriseId, siteName)

	value, err := rdb.Get(ctx, key).Result()

	if errors.Is(err, redis.Nil) { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("[GetSiteIDFromCache] error getting key from redis")
		return
	} else if value == NullStr {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		var rawSiteId int
		rawSiteId, err = strconv.Atoi(value)
		if err != nil {
			zap.S().Errorf("[GetSiteIDFromCache] error converting value to integer")
			return
		}

		var int32SiteId int32
		int32SiteId, err = safecast.Int32(rawSiteId)
		if err != nil {
			zap.S().Errorf("[GetSiteIDFromCache] error converting value to integer")
			return
		}

		siteId = uint32(int32SiteId)

		cacheHit = true
	}
	return
}

// StoreAreaIDToCache stores area id to cache
func StoreAreaIDToCache(siteId uint32, areaName string, areaID uint32) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	strSiteId := strconv.Itoa(int(siteId))

	key := fmt.Sprintf("getAreaName-%s-%s", strSiteId, areaName)

	if areaID == 0 {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	b := strconv.Itoa(int(areaID))

	err := RedisSafeSet(ctx, key, []byte(b), redisDataExpiration)
	if err != nil {
		zap.S().Errorf("redis failed: %#v", err)
		return
	}
}

// GetAreaIDFromCache gets area id from cache
func GetAreaIDFromCache(siteId uint32, areaName string) (areaId uint32, cacheHit bool) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	strSiteId := strconv.Itoa(int(siteId))

	key := fmt.Sprintf("getAreaName-%s-%s", strSiteId, areaName)

	value, err := rdb.Get(ctx, key).Result()

	if errors.Is(err, redis.Nil) { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("[GetAreaIDFromCache] error getting key from redis")
		return
	} else if value == NullStr {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		var rawAreaId int
		rawAreaId, err = strconv.Atoi(value)
		if err != nil {
			zap.S().Errorf("[GetAreaIDFromCache] error converting value to integer")
			return
		}

		var int32AreaId int32
		int32AreaId, err = safecast.Int32(rawAreaId)
		if err != nil {
			zap.S().Errorf("[GetAreaIDFromCache] error converting value to integer")
			return
		}

		areaId = uint32(int32AreaId)

		cacheHit = true
	}
	return
}

// StoreProductionLineIDToCache stores production line id to cache
func StoreProductionLineIDToCache(areaId uint32, productionLineName string, productionLineID uint32) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	strAreaId := strconv.Itoa(int(areaId))

	key := fmt.Sprintf("getProductionLineName-%s-%s", strAreaId, productionLineName)

	if productionLineID == 0 {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	b := strconv.Itoa(int(productionLineID))

	err := RedisSafeSet(ctx, key, []byte(b), redisDataExpiration)
	if err != nil {
		zap.S().Errorf("redis failed: %#v", err)
		return
	}
}

// GetProductionLineIDFromCache gets production line id from cache
func GetProductionLineIDFromCache(areaId uint32, productionLineName string) (productionLineId uint32, cacheHit bool) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	strAreaId := strconv.Itoa(int(areaId))

	key := fmt.Sprintf("getProductionLineName-%s-%s", strAreaId, productionLineName)

	value, err := rdb.Get(ctx, key).Result()

	if errors.Is(err, redis.Nil) { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("[GetProductionLineIDFromCache] error getting key from redis")
		return
	} else if value == NullStr {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		var rawProductionLineId int
		rawProductionLineId, err = strconv.Atoi(value)
		if err != nil {
			zap.S().Errorf("[GetProductionLineIDFromCache] error converting value to integer")
			return
		}

		var int32ProductionLineId int32
		int32ProductionLineId, err = safecast.Int32(rawProductionLineId)
		if err != nil {
			zap.S().Errorf("[GetProductionLineIDFromCache] error converting value to integer")
			return
		}

		productionLineId = uint32(int32ProductionLineId)

		cacheHit = true
	}
	return
}

// StoreWorkCellIDToCache stores work cell id to cache
func StoreWorkCellIDToCache(productionLineId uint32, workCellName string, workCellID uint32) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	strProductionLineId := strconv.Itoa(int(productionLineId))

	key := fmt.Sprintf("getWorkCellName-%s-%s", strProductionLineId, workCellName)

	if workCellID == 0 {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	b := strconv.Itoa(int(workCellID))

	err := RedisSafeSet(ctx, key, []byte(b), redisDataExpiration)
	if err != nil {
		zap.S().Errorf("redis failed: %#v", err)
		return
	}
}

// GetWorkCellIDFromCache gets work cell id from cache
func GetWorkCellIDFromCache(productionLineId uint32, workCellName string) (workCellId uint32, cacheHit bool) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	strProductionLineId := strconv.Itoa(int(productionLineId))

	key := fmt.Sprintf("getWorkCellName-%s-%s", strProductionLineId, workCellName)

	value, err := rdb.Get(ctx, key).Result()

	if errors.Is(err, redis.Nil) { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("[GetWorkCellIDFromCache] error getting key from redis")
		return
	} else if value == NullStr {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		var rawWorkCellId int
		rawWorkCellId, err = strconv.Atoi(value)
		if err != nil {
			zap.S().Errorf("[GetWorkCellIDFromCache] error converting value to integer")
			return
		}

		var int32WorkCellId int32
		int32WorkCellId, err = safecast.Int32(rawWorkCellId)
		if err != nil {
			zap.S().Errorf("[GetWorkCellIDFromCache] error converting value to integer")
			return
		}

		workCellId = uint32(int32WorkCellId)

		cacheHit = true
	}
	return
}

// StoreEnterpriseConfigurationToCache stores customer id to cache
func StoreEnterpriseConfigurationToCache(enterpriseName string, data datamodel.EnterpriseConfiguration) {

	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("GetEnterpriseConfigurationFromCache-%s", enterpriseName)

	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	b, err := json.Marshal(&data)
	if err != nil {
		zap.S().Errorf("json marshall")
		return
	}

	err = RedisSafeSet(ctx, key, b, redisDataExpiration)
	if err != nil {
		zap.S().Errorf("redis failed")
		zap.S().Errorf("redis failed: %#v", err)
		return
	}
}

// GetEnterpriseConfigurationFromCache gets customer configuration from cache
func GetEnterpriseConfigurationFromCache(enterpriseName string) (data datamodel.EnterpriseConfiguration, cacheHit bool) {
	if rdb == nil { // only the case during tests
		// zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("GetEnterpriseConfigurationFromCache-%s", enterpriseName)

	value, err := rdb.Get(ctx, key).Result()

	if errors.Is(err, redis.Nil) { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == NullStr {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		// https://itnext.io/storing-go-structs-in-redis-using-rejson-dab7f8fc0053
		b := []byte(value)
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		err = json.Unmarshal(b, &data)
		if err != nil {
			zap.S().Errorf("json Unmarshal", b)
			return
		}

		cacheHit = true
	}

	return
}

// GetTiered Attempts to get key from memory cache, if fails it falls back to redis
func GetTiered(key string) (cached bool, value interface{}) {
	if memCache == nil && rdb == nil {
		return false, 0
	}
	// Check if in memCache
	value, cached = memCache.Get(key)
	if cached {
		zap.S().Infof("Found in memcache")
		return
	}

	var err error
	// Check if in redis
	d := time.Now().Add(memoryDataExpiration)
	var cancel context.CancelFunc
	ctx, cancel = context.WithDeadline(context.Background(), d)
	defer cancel()

	value, err = rdb.Get(ctx, key).Bytes()
	if err != nil {
		zap.S().Infof("Not found in redis")
		return false, nil
	}
	cached = true
	zap.S().Infof("Found in redis")

	// Write back to memCache
	memCache.SetDefault(key, value)
	return
}

func SetMemcached(key string, value interface{}) {
	memCache.SetDefault(key, value)
}

func GetMemcached(key string) (value interface{}, found bool) {
	value, found = memCache.Get(key)
	return
}

func SetMemcachedLong(key string, value interface{}, d time.Duration) {
	memCache.Set(key, value, d)
}

func RedisSafeSet(ctx context.Context, k string, v []byte, t time.Duration) error {
	// If v > 512MB, don't store it in redis
	// To be safe, we'll use 500MB as a cutoff
	if len(v) > 500*1024*1024 {
		zap.S().Warnf("RedisSafeSet: value too large to store in redis (%s)", k)
		return nil
	}
	err := rdb.Set(ctx, k, v, t).Err()
	return err
}
