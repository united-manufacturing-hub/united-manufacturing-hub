package postgresql

import (
	"encoding/binary"
	"errors"
	"github.com/jackc/pgx/v5"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"golang.org/x/crypto/sha3"
	"strings"
	"sync"
)

var goiLock = sync.Mutex{}

func (c *Connection) GetOrInsertAsset(topic *sharedStructs.TopicDetails) (uint64, error) {
	if c.Db == nil {
		return 0, errors.New("database is nil")
	}
	id, hit := c.lookupAssetIdLRU(topic)
	if hit {
		return id, nil
	}

	goiLock.Lock()
	defer goiLock.Unlock()
	// It might be that another locker already added it, so we do a double check
	id, hit = c.lookupAssetIdLRU(topic)
	if hit {
		return id, nil
	}

	selectQuery := `SELECT id FROM asset WHERE enterprise = $1 AND 
    site = $2 AND 
    area = $3 AND 
    line = $4 AND 
    workcell = $5 AND 
    origin_id = $6`
	selectRowContext, selectRowContextCncl := get1MinuteContext()
	defer selectRowContextCncl()

	var err error
	var idx int
	err = c.Db.QueryRow(selectRowContext, selectQuery, topic.Enterprise, topic.Site, topic.Area, topic.ProductionLine, topic.WorkCell, topic.OriginId).Scan(&idx)
	id = uint64(idx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Row isn't found, need to insert
			insertQuery := `INSERT INTO asset (enterprise, site, area, line, workcell, origin_id) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
			insertRowContext, insertRowContextCncl := get1MinuteContext()
			defer insertRowContextCncl()

			err = c.Db.QueryRow(insertRowContext, insertQuery, topic.Enterprise, topic.Site, topic.Area, topic.ProductionLine, topic.WorkCell, topic.OriginId).Scan(&id)
			if err != nil {
				return 0, err
			}

			// Add to LRU cache
			c.addToAssetIdLRU(topic, id)

			return id, nil
		} else {
			return 0, err
		}
	}

	// If no error and asset exists
	c.addToAssetIdLRU(topic, id)
	return id, nil
}

func (c *Connection) addToAssetIdLRU(topic *sharedStructs.TopicDetails, id uint64) {
	c.AssetIdCache.Add(getCacheKeyFromTopic(topic), id)
}

func (c *Connection) lookupAssetIdLRU(topic *sharedStructs.TopicDetails) (uint64, bool) {
	value, ok := c.AssetIdCache.Get(getCacheKeyFromTopic(topic))
	if ok {
		c.lruHits.Add(1)
		return value.(uint64), true
	}
	c.lruMisses.Add(1)
	return 0, false
}

var ptIdLock = sync.Mutex{}

func (c *Connection) GetOrInsertProductType(assetId uint64, externalProductId string, cycleTimeMs uint64) (uint64, error) {
	if c.Db == nil {
		return 0, errors.New("database is nil")
	}
	ptId, hit := c.lookupProductTypeIdLRU(assetId, externalProductId)
	if hit {
		return ptId, nil
	}

	ptIdLock.Lock()
	defer ptIdLock.Unlock()

	// It might be that another locker already added it, so we do a double check
	ptId, hit = c.lookupProductTypeIdLRU(assetId, externalProductId)
	if hit {
		return ptId, nil
	}

	// Don't add if cycleTimeMs is 0
	if cycleTimeMs == 0 {
		return 0, errors.New("not found")
	}

	selectQuery := `SELECT product_type_id FROM product_type WHERE external_product_type_id = $1 AND asset_id = $2`
	selectRowContext, selectRowContextCncl := get1MinuteContext()
	defer selectRowContextCncl()

	var err error
	var ptIdX int
	err = c.Db.QueryRow(selectRowContext, selectQuery, externalProductId, int(assetId)).Scan(&ptIdX)
	ptId = uint64(ptIdX)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Row isn't found, need to insert
			insertQuery := `INSERT INTO product_type(external_product_type_id, cycle_time_ms, asset_id) VALUES ($1, $2, $3) RETURNING product_type_id`
			insertRowContext, insertRowContextCncl := get1MinuteContext()
			defer insertRowContextCncl()

			err = c.Db.QueryRow(insertRowContext, insertQuery, externalProductId, cycleTimeMs, int(assetId)).Scan(&ptId)
			if err != nil {
				return 0, err
			}

			// Add to LRU cache
			c.addToProductTypeIdLRU(assetId, externalProductId, ptId)

			return ptId, nil
		} else {
			return 0, err
		}
	}

	// If no error and externalProductId type exists
	c.addToProductTypeIdLRU(assetId, externalProductId, ptId)
	return ptId, nil
}

func (c *Connection) addToProductTypeIdLRU(assetId uint64, externalProductId string, ptId uint64) {
	c.ProductTypeIdCache.Add(getCacheKeyFromProduct(assetId, externalProductId), ptId)
}

func (c *Connection) lookupProductTypeIdLRU(assetId uint64, externalProductId string) (uint64, bool) {
	value, ok := c.ProductTypeIdCache.Get(getCacheKeyFromProduct(assetId, externalProductId))
	if ok {
		c.lruHits.Add(1)
		return value.(uint64), true
	}
	c.lruMisses.Add(1)
	return 0, false
}

func getCacheKeyFromTopic(topic *sharedStructs.TopicDetails) string {
	// Attempt cache lookup
	var cacheKey strings.Builder
	cacheKey.WriteString(topic.Enterprise)
	cacheKey.WriteRune('*') // This char cannot occur in the topic, and therefore can be safely used as a seperator
	cacheKey.WriteRune('s')
	cacheKey.WriteString(topic.Site)

	cacheKey.WriteRune('*') // This char cannot occur in the topic, and therefore can be safely used as a seperator
	cacheKey.WriteRune('a')
	cacheKey.WriteString(topic.Area)

	cacheKey.WriteRune('*') // This char cannot occur in the topic, and therefore can be safely used as a seperator
	cacheKey.WriteRune('p')
	cacheKey.WriteString(topic.ProductionLine)

	cacheKey.WriteRune('*') // This char cannot occur in the topic, and therefore can be safely used as a seperator
	cacheKey.WriteRune('w')
	cacheKey.WriteString(topic.WorkCell)

	cacheKey.WriteRune('*') // This char cannot occur in the topic, and therefore can be safely used as a seperator
	cacheKey.WriteRune('o')
	cacheKey.WriteString(topic.OriginId)
	return cacheKey.String()
}

func getCacheKeyFromProduct(assetId uint64, externalProductId string) string {
	// Attempt cache lookup
	hasher := sha3.New512()
	assetIdBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(assetIdBytes, assetId)
	hasher.Write(assetIdBytes)
	hasher.Write([]byte(externalProductId))

	return string(hasher.Sum(nil))
}
