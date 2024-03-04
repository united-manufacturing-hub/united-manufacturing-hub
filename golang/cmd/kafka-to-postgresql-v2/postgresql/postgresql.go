package postgresql

import (
	"context"
	"errors"
	"fmt"
	lru "github.com/hashicorp/golang-lru"
	"github.com/heptiolabs/healthcheck"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/united-manufacturing-hub/umh-utils/env"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"go.uber.org/zap"
	"sync"
	"sync/atomic"
	"time"
)

type DBValue struct {
	Timestamp time.Time
	Origin    string
	Value     sharedStructs.HistorianValue
	AssetId   int
}

func (r *DBValue) GetValue() interface{} {
	if r.Value.IsNumeric {
		return *r.Value.NumericValue
	} else {
		return *r.Value.StringValue
	}
}

type Metrics struct {
	LRUHitPercentage                    float64
	NumericalChannelFillPercentage      float64
	StringChannelFillPercentage         float64
	DatabaseInsertions                  uint64
	AverageCommitDurationInMilliseconds float64
	NumericalValuesReceivedPerSecond    float64
	StringValuesReceivedPerSecond       float64
	DatabaseInsertionRate               float64
}

type Connection struct {
	db                     *pgxpool.Pool
	assetIdCache           *lru.ARCCache
	productTypeIdCache     *lru.ARCCache
	numericalValuesChannel chan DBValue
	stringValuesChannel    chan DBValue
	numericalReceived      atomic.Uint64
	stringsReceived        atomic.Uint64
	databaseInserted       atomic.Uint64
	lruHits                atomic.Uint64
	lruMisses              atomic.Uint64
	commits                atomic.Uint64
	commitTime             atomic.Uint64
	metrics                Metrics
	metricsLock            sync.RWMutex
}

var conn *Connection
var once sync.Once

func GetOrInit() *Connection {
	once.Do(func() {
		zap.S().Debugf("Setting up postgresql")
		// Postgres
		PQHost, err := env.GetAsString("POSTGRES_HOST", false, "db")
		if err != nil {
			zap.S().Fatalf("Failed to get POSTGRES_HOST from env: %s", err)
		}
		PQPort, err := env.GetAsInt("POSTGRES_PORT", false, 5432)
		if err != nil {
			zap.S().Fatalf("Failed to get POSTGRES_PORT from env: %s", err)
		}
		PQUser, err := env.GetAsString("POSTGRES_USER", true, "")
		if err != nil {
			zap.S().Fatalf("Failed to get POSTGRES_USER from env: %s", err)
		}
		PQPassword, err := env.GetAsString("POSTGRES_PASSWORD", true, "")
		if err != nil {
			zap.S().Fatalf("Failed to get POSTGRES_PASSWORD from env: %s", err)
		}
		PQDBName, err := env.GetAsString("POSTGRES_DATABASE", true, "")
		if err != nil {
			zap.S().Fatalf("Failed to get POSTGRES_DATABASE from env: %s", err)
		}
		PQSSLMode, err := env.GetAsString("POSTGRES_SSL_MODE", false, "require")
		if err != nil {
			zap.S().Fatalf("Failed to get POSTGRES_SSL_MODE from env: %s", err)
		}

		zap.S().Infof("Connecting to %s@%s:%d/%s [%s]", PQUser, PQHost, PQPort, PQDBName, PQSSLMode)

		conString := fmt.Sprintf("host=%s port=%d user =%s password=%s dbname=%s sslmode=%s", PQHost, PQPort, PQUser, PQPassword, PQDBName, PQSSLMode)

		extablishContext, extablishContextCncl := get5SecondContext()
		defer extablishContextCncl()
		var db *pgxpool.Pool
		db, err = pgxpool.New(extablishContext, conString)
		if err != nil {
			zap.S().Fatalf("Failed to open connection to postgres database: %s", err)
		}

		PQLRUSize, err := env.GetAsInt("POSTGRES_LRU_CACHE_SIZE", false, 1000)
		if err != nil {
			zap.S().Fatalf("Failed to get POSTGRES_LRU_CACHE_SIZE from env: %s", err)
		}
		assetIdCache, err := lru.NewARC(PQLRUSize)
		if err != nil {
			zap.S().Fatalf("Failed to create ARC (assetId): %s", err)
		}
		ChannelSize, err := env.GetAsInt("VALUE_CHANNEL_SIZE", false, 10000)
		if err != nil {
			zap.S().Fatalf("Failed to get VALUE_CHANNEL_SIZE from env: %s", err)
		}
		productTypeIdCache, err := lru.NewARC(50)
		if err != nil {
			zap.S().Fatalf("Failed to create ARC (productTypeId): %s", err)
		}
		conn = &Connection{
			db:                     db,
			assetIdCache:           assetIdCache,
			productTypeIdCache:     productTypeIdCache,
			numericalValuesChannel: make(chan DBValue, ChannelSize),
			stringValuesChannel:    make(chan DBValue, ChannelSize),
		}
		if !conn.IsAvailable() {
			zap.S().Fatalf("Database is not available !")
		}

		// Validate that tables exist
		contextCheckTables, contextCheckTablesCncl := get5SecondContext()
		defer contextCheckTablesCncl()
		tablesToCheck := []string{"asset", "tag", "tag_string"}
		for _, table := range tablesToCheck {
			var tableName string
			query := `SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1`
			row := db.QueryRow(contextCheckTables, query, table)
			err := row.Scan(&tableName)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					zap.S().Fatalf("Table %s does not exist in the database", table)
				} else {
					zap.S().Fatalf("Failed to check for table %s: %s", table, err)
				}
			}
		}

		go conn.tagWorker("tag", &Source{
			datachan:  conn.numericalValuesChannel,
			isNumeric: true,
		})
		go conn.tagWorker("tag_string", &Source{
			datachan:  conn.stringValuesChannel,
			isNumeric: false,
		})

		go conn.postStats()
	})
	return conn
}

func (c *Connection) IsAvailable() bool {
	if c.db == nil {
		return false
	}
	ctx, cncl := get5SecondContext()
	defer cncl()
	err := c.db.Ping(ctx)
	if err != nil {
		zap.S().Debugf("Failed to ping database: %s", err)
		return false
	}
	return true
}

func (c *Connection) postStats() {
	startTime := time.Now()

	tenSecondTicker := time.NewTicker(10 * time.Second)
	for {
		<-tenSecondTicker.C
		currentNumericalReceived := c.numericalReceived.Load()
		currentStringsReceived := c.stringsReceived.Load()
		currentDatabaseInserted := c.databaseInserted.Load()

		// Calculating LRU hit percentage
		lruHits := c.lruHits.Load()
		lruMisses := c.lruMisses.Load()
		totalLRUAccesses := lruHits + lruMisses
		lruHitPercentage := 0.0
		if totalLRUAccesses > 0 {
			lruHitPercentage = float64(lruHits) / float64(totalLRUAccesses) * 100
		}

		elapsedTime := time.Since(startTime).Seconds()
		numericalRate := float64(currentNumericalReceived) / elapsedTime
		stringRate := float64(currentStringsReceived) / elapsedTime
		databaseInsertionRate := float64(currentDatabaseInserted) / elapsedTime

		numericalChannelFill := len(c.numericalValuesChannel)
		numericalChannelFillPercentage := float64(0)
		if numericalChannelFill > 0 {
			numericalChannelFillPercentage = float64(numericalChannelFill) / float64(cap(c.numericalValuesChannel)) * 100
		}

		stringsChannelFill := len(c.stringValuesChannel)
		stringsChannelFillPercentage := float64(0)
		if stringsChannelFill > 0 {
			stringsChannelFillPercentage = float64(stringsChannelFill) / float64(cap(c.stringValuesChannel)) * 100
		}

		totalCommits := c.commits.Load()
		totalCommitTime := c.commitTime.Load() // Assuming this is in milliseconds

		averageCommitDuration := float64(0)
		if totalCommits > 0 {
			averageCommitDuration = float64(totalCommitTime) / float64(totalCommits)
		}
		c.metricsLock.Lock()
		c.metrics = Metrics{
			LRUHitPercentage:                    lruHitPercentage,
			NumericalChannelFillPercentage:      numericalChannelFillPercentage,
			StringChannelFillPercentage:         stringsChannelFillPercentage,
			DatabaseInsertions:                  c.databaseInserted.Load(),
			DatabaseInsertionRate:               databaseInsertionRate,
			AverageCommitDurationInMilliseconds: averageCommitDuration,
			NumericalValuesReceivedPerSecond:    numericalRate,
			StringValuesReceivedPerSecond:       stringRate,
		}
		c.metricsLock.Unlock()

		// Check if there were no database insertions
		if currentDatabaseInserted == 0 {
			zap.S().Info("No database insertions so far")
		}
	}
}

func (c *Connection) GetMetrics() Metrics {
	c.metricsLock.RLock()
	c.metricsLock.RUnlock()
	return c.metrics
}

func GetHealthCheck() healthcheck.Check {
	return func() error {
		if GetOrInit().IsAvailable() {
			return nil
		} else {
			return errors.New("healthcheck failed to reach database")
		}
	}
}

func get5SecondContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func get1MinuteContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 1*time.Minute)
}

// calculateSleepTime scales the sleep time based on the number of entries
func (c *Connection) calculateSleepTime(copiedIn int64) time.Duration {
	const maxSleep = 5 * time.Second
	const minEntries = 1
	maxEntries := int64(cap(c.numericalValuesChannel))

	if copiedIn >= maxEntries {
		return 0 // or a very small duration
	}

	// Linear scaling of sleep time
	sleepScale := float64(maxSleep) * (1 - float64(copiedIn)/float64(maxEntries))
	if sleepScale < 0 {
		sleepScale = 0
	}

	return time.Duration(sleepScale)
}
