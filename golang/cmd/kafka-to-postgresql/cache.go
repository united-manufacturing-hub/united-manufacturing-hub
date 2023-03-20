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

package main

import (
	"encoding/binary"
	"github.com/coocood/freecache"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

// dbcache is a cache of database entries (for example database asset id's)
var dbcache *freecache.Cache

// InitCache initializes the cache with the given sizes in byte
func InitCache(DBcacheSizeBytes int) {
	zap.S().Infof("Initializing cache with DB cache size %d bytes", DBcacheSizeBytes)
	dbcache = freecache.NewCache(DBcacheSizeBytes)
}

// GetUint32DB looks up the DB cache for the key and returns the value if found as a uint32
func GetUint32DB(key []byte) (uint32, bool) {
	if dbcache == nil {
		return 0, false
	}
	value, err := dbcache.Get(key)
	if err != nil {
		return 0, false
	}
	return BytesToUint32(value), true
}

// PutUint32DB puts the key and value into the DB cache
func PutUint32DB(key []byte, value uint32) {
	if dbcache == nil {
		return
	}
	err := dbcache.Set(key, Uint32ToBytes(value), 0)
	if err != nil {
		zap.S().Errorf("Error putting key %s into cache: %s", key, err)
	}
}

// Uint32ToBytes converts a uint32 to a byte array
func Uint32ToBytes(a uint32) (b []byte) {
	b = make([]byte, 4)
	binary.LittleEndian.PutUint32(b, a)
	return
}

// BytesToUint32 converts a byte array to a uint32
func BytesToUint32(b []byte) (a uint32) {
	return binary.LittleEndian.Uint32(b)
}

// GetCacheAssetTableId looks up the db cache for the customerId, locationId, assetId and returns the asset table id if found
func GetCacheAssetTableId(customerID string, locationID string, assetID string) (uint32, bool) {
	return GetUint32DB(
		internal.AsXXHash(
			[]byte("AssetTableId"),
			[]byte(customerID),
			[]byte(locationID),
			[]byte(assetID)))
}

// PutCacheAssetTableId puts the assetTableId into the db cache, using the customerId, locationId, assetId as the key
func PutCacheAssetTableId(customerID string, locationID string, assetID string, assetTableId uint32) {
	PutUint32DB(
		internal.AsXXHash([]byte("AssetTableId"), []byte(customerID), []byte(locationID), []byte(assetID)),
		assetTableId)
}

// PutCacheProductTableId puts the productTableId into the db cache, using the customerId, locationId, productId as the key
func PutCacheProductTableId(customerID string, AssetTableId uint32, productTableId uint32) {
	PutUint32DB(
		internal.AsXXHash([]byte("ProductTableId"), []byte(customerID), Uint32ToBytes(AssetTableId)),
		productTableId)
}

// GetCacheProductTableId looks up the db cache for the customerId, locationId, productId and returns the product table id if found
func GetCacheProductTableId(customerID string, AssetTableId uint32) (uint32, bool) {
	return GetUint32DB(internal.AsXXHash([]byte("ProductTableId"), []byte(customerID), Uint32ToBytes(AssetTableId)))
}

// GetCacheUniqueProductTableId looks up the db cache for the customerId, locationId, productId and returns the product table id if found
func GetCacheUniqueProductTableId(UniqueProductAlternativeId string, AssetTableId uint32) (uint32, bool) {
	return GetUint32DB(
		internal.AsXXHash(
			[]byte("UniqueProductTableId"),
			[]byte(UniqueProductAlternativeId),
			Uint32ToBytes(AssetTableId)))
}

// PutCacheUniqueProductTableId puts the productTableId into the db cache, using the customerId, locationId, productId as the key
func PutCacheUniqueProductTableId(UniqueProductAlternativeId string, AssetTableId uint32, UniqueProductTableId uint32) {
	PutUint32DB(
		internal.AsXXHash(
			[]byte("UniqueProductTableId"),
			[]byte(UniqueProductAlternativeId),
			Uint32ToBytes(AssetTableId)), UniqueProductTableId)
}

func GetCacheLatestParentUniqueProductID(ParentID string, AssetTableId uint32) (uint32, bool) {
	return GetUint32DB(
		internal.AsXXHash(
			[]byte("LatestParentUniqueProductID"),
			[]byte(ParentID),
			Uint32ToBytes(AssetTableId)))
}

func PutCacheLatestParentUniqueProductID(ParentID string, AssetTableId uint32, UniqueProductTableId uint32) {
	PutUint32DB(
		internal.AsXXHash([]byte("LatestParentUniqueProductID"), []byte(ParentID), Uint32ToBytes(AssetTableId)),
		UniqueProductTableId)
}
