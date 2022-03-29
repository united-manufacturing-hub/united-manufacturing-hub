package internal

import (
	"fmt"
	"hash/crc32"
	"time"

	"context"
	"github.com/go-redis/redis/v8"
	"github.com/patrickmn/go-cache"
	"go.uber.org/zap"
)

var rdb *redis.Client
var ctx = context.Background()
var memCache *cache.Cache

var redisDataExpiration time.Duration
var memoryDataExpiration time.Duration

var redisInitialized bool

// InitCache initializes a redis cache
func InitCache(redisURI string, redisURI2 string, redisURI3 string, redisPassword string, redisDB int, dryRun string) {

	if dryRun == "True" || dryRun == "true" {
		zap.S().Infof("Running cache in DRY_RUN mode. This means that cache will not be used") // "... and it stays nil"
		return
	}

	var failOverOptions = redis.FailoverOptions{
		MasterName:       "mymaster",
		SentinelAddrs:    []string{redisURI, redisURI2, redisURI3},
		SentinelPassword: redisPassword,
		Password:         redisPassword,
		DB:               redisDB,
	}
	zap.S().Debugf("Initializing redis cache with options: %#v", failOverOptions)

	rdb = redis.NewFailoverClient(&failOverOptions)

	redisDataExpiration = 12 * time.Hour
	memoryDataExpiration = 10 * time.Second

	memCache = cache.New(memoryDataExpiration, 20*time.Second)
	redisInitialized = true
}

func InitMemcache() {
	memoryDataExpiration = 10 * time.Second
	memCache = cache.New(memoryDataExpiration, 20*time.Second)
	redisInitialized = false
}

func IsRedisAvailable() bool {
	if !redisInitialized {
		zap.S().Warn("Redis is not initialized")
		return false
	}
	if rdb != nil {
		zap.S().Debugf("Checking if redis is available")
		timeout, _ := context.WithTimeout(ctx, time.Second*10)
		statusCmd := rdb.Ping(timeout)

		if statusCmd != nil && statusCmd.Val() == "PONG" {
			zap.S().Debugf("Redis is available")
			return true
		}
		zap.S().Debugf("Redis Error: %s", statusCmd)
	}
	return false
}

// AsHash returns a hash for a given interface
func AsHash(o interface{}) string {
	h := crc32.NewIEEE() // modified for quicker hashing
	// This cannot fail
	_, _ = h.Write([]byte(fmt.Sprintf("%v", o)))

	return fmt.Sprintf("%x", h.Sum(nil))
}

// GetTiered Attempts to get key from memory cache, if fails it falls back to redis
func GetTiered(key string) (cached bool, value interface{}) {
	//Check if in memCache
	value, cached = memCache.Get(key)
	if cached {
		zap.S().Infof("Found in memcache")
		return
	}

	var err error
	//Check if in redis
	d := time.Now().Add(memoryDataExpiration)
	ctx, cancel := context.WithDeadline(context.Background(), d)
	defer cancel()

	value, err = rdb.Get(ctx, key).Bytes()
	if err != nil {
		zap.S().Infof("Not found in redis")
		return false, nil
	}
	cached = true
	zap.S().Infof("Found in redis")

	//Write back to memCache
	memCache.SetDefault(key, value)
	return
}

// SetTiered sets memcache and redis with expiration
func SetTiered(key string, value interface{}, redisExpiration time.Duration) {
	memCache.SetDefault(key, value)
	rdb.Set(ctx, key, value, redisExpiration)
}

//SetTieredLongTerm is an helper, that calls SetTiered with default redis expiration
func SetTieredLongTerm(key string, value interface{}) {
	SetTiered(key, value, redisDataExpiration)
}

//SetTieredShortTerm is an helper, that calls SetTiered with default memory expiration
func SetTieredShortTerm(key string, value interface{}) {
	SetTiered(key, value, memoryDataExpiration)
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
