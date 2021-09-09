package internal

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"strconv"
	"time"

	"context"
	"github.com/go-redis/redis/v8"
	"github.com/rung/go-safecast"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
)

var rdb *redis.Client
var ctx = context.Background()

var dataExpiration time.Duration

// InitCache initializes a redis cache
func InitCache(redisURI string, redisURI2 string, redisURI3 string, redisPassword string, redisDB int, dryRun string) {

	if dryRun == "True" || dryRun == "true" {
		zap.S().Infof("Running cache in DRY_RUN mode. This means that cache will not be used") // "... and it stays nil"
		return
	}
	rdb = redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:       "mymaster",
		SentinelAddrs:    []string{redisURI, redisURI2, redisURI3},
		SentinelPassword: redisPassword,
		Password:         redisPassword,
		DB:               redisDB,
	})

	dataExpiration = 12 * time.Hour
}

// AsHash returns a hash for a given interface
func AsHash(o interface{}) string {
	h := crc32.NewIEEE() // modified for quicker hashing
	h.Write([]byte(fmt.Sprintf("%v", o)))

	return fmt.Sprintf("%x", h.Sum(nil))
}

// GetProcessStatesFromCache gets process states from cache
func GetProcessStatesFromCache(key string) (processedStateArray []datamodel.StateEntry, cacheHit bool) {
	if rdb == nil { // only the case during tests
		////zap.S().Errorf("rdb == nil")
		return
	}

	value, err := rdb.Get(ctx, key).Result()

	if err == redis.Nil { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == "null" {
		//zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		// https://itnext.io/storing-go-structs-in-redis-using-rejson-dab7f8fc0053
		b := []byte(value)
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

	b, err := json.Marshal(&processedStateArray)
	if err != nil {
		zap.S().Errorf("json marshall")
		return
	}

	err = rdb.Set(ctx, key, b, dataExpiration).Err()
	if err != nil {
		zap.S().Errorf("redis failed")
		return
	}
}

// GetCalculatateLowSpeedStatesFromCache get low speed states from cache
func GetCalculatateLowSpeedStatesFromCache(from time.Time, to time.Time, assetID uint32) (processedStateArray []datamodel.StateEntry, cacheHit bool) {
	if rdb == nil { // only the case during tests
		////zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("CalculatateLowSpeedStates-%s-%s-%d", from, to, assetID)

	value, err := rdb.Get(ctx, key).Result()

	if err == redis.Nil { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == "null" {
		//zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		// https://itnext.io/storing-go-structs-in-redis-using-rejson-dab7f8fc0053
		b := []byte(value)
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
func StoreCalculatateLowSpeedStatesToCache(from time.Time, to time.Time, assetID uint32, processedStateArray []datamodel.StateEntry) {
	if rdb == nil { // only the case during tests
		////zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("CalculatateLowSpeedStates-%s-%s-%d", from, to, assetID)

	if processedStateArray == nil {
		//zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	b, err := json.Marshal(&processedStateArray)
	if err != nil {
		zap.S().Errorf("json marshall: %+v", err)
		return
	}

	err = rdb.Set(ctx, key, b, dataExpiration).Err()
	if err != nil {
		zap.S().Errorf("redis failed: %+v", err)
		return
	}
}

// GetStatesRawFromCache gets raw states from cache
func GetStatesRawFromCache(assetID uint32, from time.Time, to time.Time, configuration datamodel.CustomerConfiguration) (data []datamodel.StateEntry, cacheHit bool) {
	if rdb == nil { // only the case during tests
		////zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getStatesRawFromCache-%d-%s-%s-%s", assetID, from, to, AsHash(configuration))

	value, err := rdb.Get(ctx, key).Result()

	if err == redis.Nil { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == "null" {
		//zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		// https://itnext.io/storing-go-structs-in-redis-using-rejson-dab7f8fc0053
		b := []byte(value)
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
func StoreRawStatesToCache(assetID uint32, from time.Time, to time.Time, configuration datamodel.CustomerConfiguration, data []datamodel.StateEntry) {
	if rdb == nil { // only the case during tests
		//zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getStatesRawFromCache-%d-%s-%s-%s", assetID, from, to, AsHash(configuration))

	if data == nil {
		//zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	b, err := json.Marshal(&data)
	if err != nil {
		zap.S().Errorf("json marshall")
		return
	}

	err = rdb.Set(ctx, key, b, dataExpiration).Err()
	if err != nil {
		zap.S().Errorf("redis failed")
		return
	}
}

// GetRawShiftsFromCache gets raw shifts from cache
func GetRawShiftsFromCache(assetID uint32, from time.Time, to time.Time, configuration datamodel.CustomerConfiguration) (data []datamodel.ShiftEntry, cacheHit bool) {
	if rdb == nil { // only the case during tests
		//zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getRawShiftsFromCache-%d-%s-%s-%s", assetID, from, to, AsHash(configuration))

	value, err := rdb.Get(ctx, key).Result()

	if err == redis.Nil { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == "null" {
		//zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		// https://itnext.io/storing-go-structs-in-redis-using-rejson-dab7f8fc0053
		b := []byte(value)
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
func StoreRawShiftsToCache(assetID uint32, from time.Time, to time.Time, configuration datamodel.CustomerConfiguration, data []datamodel.ShiftEntry) {
	if rdb == nil { // only the case during tests
		//zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getRawShiftsFromCache-%d-%s-%s-%s", assetID, from, to, AsHash(configuration))

	if data == nil {
		//zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	b, err := json.Marshal(&data)
	if err != nil {
		zap.S().Errorf("json marshall")
		return
	}

	err = rdb.Set(ctx, key, b, dataExpiration).Err()
	if err != nil {
		zap.S().Errorf("redis failed")
		return
	}
}

// GetRawCountsFromCache gets raw counts from cache
func GetRawCountsFromCache(assetID uint32, from time.Time, to time.Time) (data []datamodel.CountEntry, cacheHit bool) {
	if rdb == nil { // only the case during tests
		//zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getRawCountsFromCache-%d-%s-%s", assetID, from, to)

	value, err := rdb.Get(ctx, key).Result()

	if err == redis.Nil { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == "null" {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		// https://itnext.io/storing-go-structs-in-redis-using-rejson-dab7f8fc0053
		b := []byte(value)
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
		//zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getRawCountsFromCache-%d-%s-%s", assetID, from, to)

	if data == nil {
		//zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	b, err := json.Marshal(&data)
	if err != nil {
		zap.S().Errorf("json marshall")
		return
	}

	err = rdb.Set(ctx, key, b, dataExpiration).Err()
	if err != nil {
		zap.S().Errorf("redis failed")
		return
	}
}

// GetAverageStateTimeFromCache gets average state time from cache
func GetAverageStateTimeFromCache(key string) (data []interface{}, cacheHit bool) {
	if rdb == nil { // only the case during tests
		//zap.S().Errorf("rdb == nil")
		return
	}

	value, err := rdb.Get(ctx, key).Result()

	if err == redis.Nil { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == "null" {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		// https://itnext.io/storing-go-structs-in-redis-using-rejson-dab7f8fc0053
		b := []byte(value)
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
		//zap.S().Errorf("rdb == nil")
		return
	}

	if data == nil {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	b, err := json.Marshal(&data)
	if err != nil {
		zap.S().Errorf("json marshall")
		return
	}

	err = rdb.Set(ctx, key, b, dataExpiration).Err()
	if err != nil {
		zap.S().Errorf("redis failed")
		return
	}
}

// GetDistinctProcessValuesFromCache gets distinct process values from cache
func GetDistinctProcessValuesFromCache(customerID string, location string, assetID string) (data []string, cacheHit bool) {
	if rdb == nil { // only the case during tests
		//zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getDistinctProcessValues-%s-%s-%s", customerID, location, assetID)

	value, err := rdb.Get(ctx, key).Result()

	if err == redis.Nil { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == "null" {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		// https://itnext.io/storing-go-structs-in-redis-using-rejson-dab7f8fc0053
		b := []byte(value)
		err = json.Unmarshal(b, &data)
		if err != nil {
			zap.S().Errorf("json Unmarshal", b)
			return
		}

		cacheHit = true
	}

	return
}

// StoreDistinctProcessValuesToCache stores distinct process values to cache
func StoreDistinctProcessValuesToCache(customerID string, location string, assetID string, data []string) {

	if rdb == nil { // only the case during tests
		//zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getDistinctProcessValues-%s-%s-%s", customerID, location, assetID)

	if data == nil {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	b, err := json.Marshal(&data)
	if err != nil {
		zap.S().Errorf("json marshall")
		return
	}

	err = rdb.Set(ctx, key, b, dataExpiration).Err()
	if err != nil {
		zap.S().Errorf("redis failed")
		return
	}
}

// GetCustomerConfigurationFromCache gets customer configuration from cache
func GetCustomerConfigurationFromCache(customerID string) (data datamodel.CustomerConfiguration, cacheHit bool) {
	if rdb == nil { // only the case during tests
		//zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("GetCustomerConfigurationFromCache-%s", customerID)

	value, err := rdb.Get(ctx, key).Result()

	if err == redis.Nil { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == "null" {
		// zap.S().Debugf("got empty value back from redis. Ignoring...", key)
	} else {
		// https://itnext.io/storing-go-structs-in-redis-using-rejson-dab7f8fc0053
		b := []byte(value)
		err = json.Unmarshal(b, &data)
		if err != nil {
			zap.S().Errorf("json Unmarshal", b)
			return
		}

		cacheHit = true
	}

	return
}

// StoreCustomerConfigurationToCache stores customer configuration to cache
func StoreCustomerConfigurationToCache(customerID string, data datamodel.CustomerConfiguration) {

	if rdb == nil { // only the case during tests
		//zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("GetCustomerConfigurationFromCache-%s", customerID)

	b, err := json.Marshal(&data)
	if err != nil {
		zap.S().Errorf("json marshall")
		return
	}

	err = rdb.Set(ctx, key, b, dataExpiration).Err()
	if err != nil {
		zap.S().Errorf("redis failed")
		return
	}
}

// GetAssetIDFromCache gets asset id from cache
func GetAssetIDFromCache(customerID string, location string, assetID string) (DBassetID uint32, cacheHit bool) {
	if rdb == nil { // only the case during tests
		//zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getAssetID-%s-%s-%s", customerID, location, assetID)

	value, err := rdb.Get(ctx, key).Result()

	if err == redis.Nil { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == "null" {
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
		//zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getAssetID-%s-%s-%s", customerID, location, assetID)

	if DBassetID == 0 {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	b := strconv.Itoa(int(DBassetID))

	err := rdb.Set(ctx, key, b, dataExpiration).Err()
	if err != nil {
		zap.S().Errorf("redis failed")
		return
	}
}

// GetUniqueProductIDFromCache gets uniqueProduct from cache
func GetUniqueProductIDFromCache(aid string, DBassetID uint32) (uid uint32, cacheHit bool) {
	if rdb == nil { // only the case during tests
		//zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getUniqueProductID-%s-%d", aid, DBassetID)

	value, err := rdb.Get(ctx, key).Result()

	if err == redis.Nil { // if no value, then return nothing
		return
	} else if err != nil {
		zap.S().Errorf("error getting key from redis", key, err)
		return
	} else if value == "null" {
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
		//zap.S().Errorf("rdb == nil")
		return
	}

	key := fmt.Sprintf("getUniqueProductID-%s-%d", aid, DBassetID)

	if uid == 0 {
		// zap.S().Debugf("input is empty. aborting storing into database.", key)
		return
	}

	b := strconv.Itoa(int(uid))

	err := rdb.Set(ctx, key, b, dataExpiration).Err()
	if err != nil {
		zap.S().Errorf("redis failed")
		return
	}
}
