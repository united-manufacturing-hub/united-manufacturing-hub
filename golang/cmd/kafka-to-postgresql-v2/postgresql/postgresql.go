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
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type DBValue struct {
	Timestamp time.Time
	Origin    string
	AssetId   int
	Value     *sharedStructs.Value
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
	cache                  *lru.ARCCache
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
		cache, err := lru.NewARC(PQLRUSize)
		if err != nil {
			zap.S().Fatalf("Failed to create ARC: %s", err)
		}
		ChannelSize, err := env.GetAsInt("VALUE_CHANNEL_SIZE", false, 10000)
		if err != nil {
			zap.S().Fatalf("Failed to get VALUE_CHANNEL_SIZE from env: %s", err)
		}
		conn = &Connection{
			db:                     db,
			cache:                  cache,
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

var goiLock = sync.Mutex{}

func (c *Connection) GetOrInsertAsset(topic *sharedStructs.TopicDetails) (int, error) {
	if c.db == nil {
		return 0, errors.New("database is nil")
	}
	id, hit := c.lookupLRU(topic)
	if hit {
		return id, nil
	}

	goiLock.Lock()
	defer goiLock.Unlock()
	// It might be that another locker already added it, so we do a double check
	id, hit = c.lookupLRU(topic)
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
	err = c.db.QueryRow(selectRowContext, selectQuery, topic.Enterprise, topic.Site, topic.Area, topic.ProductionLine, topic.WorkCell, topic.OriginId).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Row isn't found, need to insert
			insertQuery := `INSERT INTO asset (enterprise, site, area, line, workcell, origin_id) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
			insertRowContext, insertRowContextCncl := get1MinuteContext()
			defer insertRowContextCncl()

			err = c.db.QueryRow(insertRowContext, insertQuery, topic.Enterprise, topic.Site, topic.Area, topic.ProductionLine, topic.WorkCell, topic.OriginId).Scan(&id)
			if err != nil {
				return 0, err
			}

			// Add to LRU cache
			c.addToLRU(topic, id)

			return id, nil
		} else {
			return 0, err
		}
	}

	// If no error and asset exists
	c.addToLRU(topic, id)
	return id, nil
}

func (c *Connection) addToLRU(topic *sharedStructs.TopicDetails, id int) {
	c.cache.Add(getCacheKey(topic), id)
}

func (c *Connection) lookupLRU(topic *sharedStructs.TopicDetails) (int, bool) {
	value, ok := c.cache.Get(getCacheKey(topic))
	if ok {
		c.lruHits.Add(1)
		return value.(int), true
	}
	c.lruMisses.Add(1)
	return 0, false
}

func getCacheKey(topic *sharedStructs.TopicDetails) string {
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

func (c *Connection) InsertHistorianValue(values []sharedStructs.Value, timestampMs int64, origin string, topic *sharedStructs.TopicDetails) error {
	assetId, err := c.GetOrInsertAsset(topic)
	if err != nil {
		return err
	}
	seconds := timestampMs / 1000
	nanoseconds := (timestampMs % 1000) * 1000000
	timestamp := time.Unix(seconds, nanoseconds)
	for _, value := range values {
		valX := value // This is important, otherwise go does not bind pointers correctly!
		if value.IsNumeric {
			c.numericalValuesChannel <- DBValue{
				Timestamp: timestamp,
				Origin:    origin,
				AssetId:   assetId,
				Value:     &valX,
			}
			c.numericalReceived.Add(1)
		} else {
			c.stringValuesChannel <- DBValue{
				Timestamp: timestamp,
				Origin:    origin,
				AssetId:   assetId,
				Value:     &valX,
			}
			c.stringsReceived.Add(1)
		}
	}
	return nil
}

// Source This implementation is not thread-safe !
type Source struct {
	datachan  chan DBValue
	isNumeric bool
}

func (s *Source) Next() bool {
	return len(s.datachan) != 0
}

func (s *Source) Values() ([]any, error) {
	select {
	case msg := <-s.datachan:
		values := make([]any, 5)
		values[0] = msg.Timestamp
		values[1] = msg.Value.Name
		values[2] = msg.Origin
		values[3] = msg.AssetId
		if s.isNumeric {
			values[4] = msg.GetValue().(float32)
		} else {
			values[4] = msg.GetValue().(string)
		}
		zap.S().Debugf("Values: %+v", values)
		return values, nil
	default:
		return nil, errors.New("no more rows available")
	}
}

func (s *Source) Err() error {
	return nil
}

func (c *Connection) tagWorker(tableName string, source *Source) {
	zap.S().Debugf("Starting tagWorker for %s", tableName)
	// The context is only used for preparation, not execution!

	var err error
	tableNameTemp := fmt.Sprintf("tmp_%s", tableName)
	var copiedIn int64

	for {
		sleepTime := c.calculateSleepTime(copiedIn)
		time.Sleep(sleepTime)

		if !source.Next() {
			zap.S().Debugf("No data available for %s", tableName)
			continue
		}

		txnExecutionCtx, txnExecutionCancel := get1MinuteContext()
		// Create transaction
		var txn pgx.Tx
		txn, err = c.db.BeginTx(txnExecutionCtx, pgx.TxOptions{})
		if err != nil {
			zap.S().Errorf("Failed to create transaction: %s (%s)", err, tableName)

			txnExecutionCancel()
			continue
		}

		// Create the temp table for COPY
		_, err = txn.Exec(txnExecutionCtx, fmt.Sprintf(`
	   CREATE TEMP TABLE %s
	          ( LIKE %s INCLUDING DEFAULTS )
	          ON COMMIT DROP;
	   `, tableNameTemp, tableName))
		if err != nil {
			zap.S().Errorf("Failed to execute statementCreateTmpTag: %s (%s)", err, tableName)
			rollbackCtx, rollbackCtxCncl := get5SecondContext()
			err = txn.Rollback(rollbackCtx)
			rollbackCtxCncl()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			}

			txnExecutionCancel()
			continue
		}
		copiedIn, err = txn.CopyFrom(txnExecutionCtx, pgx.Identifier{tableNameTemp}, []string{
			"timestamp", "name", "origin", "asset_id", "value",
		}, source)

		if err != nil {
			zap.S().Warnf("Failed to execute stmtCopyToTag: %s (%s)", err, tableName)
			rollbackCtx, rollbackCtxCncl := get5SecondContext()
			err = txn.Rollback(rollbackCtx)
			rollbackCtxCncl()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			}
			txnExecutionCancel()
			continue
		}

		// Do insert via statementInsertSelect
		_, err = txn.Exec(txnExecutionCtx, fmt.Sprintf(`
				INSERT INTO %s (SELECT * FROM %s) ON CONFLICT DO NOTHING;
			`, tableName, tableNameTemp))

		if err != nil {
			zap.S().Warnf("Failed to execute stmtCopyToTag: %s (%s)", err, tableName)
			rollbackCtx, rollbackCtxCncl := get5SecondContext()
			err = txn.Rollback(rollbackCtx)
			rollbackCtxCncl()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			}
			txnExecutionCancel()
			continue
		}

		zap.S().Debugf("Pre-commit")
		now := time.Now()
		err = txn.Commit(txnExecutionCtx)
		c.commits.Add(1)
		c.commitTime.Add(uint64(time.Since(now).Milliseconds()))
		zap.S().Debugf("Committing to postgresql took: %s", time.Since(now))

		if err != nil {
			zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			txnExecutionCancel()
			continue
		}
		zap.S().Debugf("Inserted %d values inside the %s table", copiedIn, tableName)
		c.databaseInserted.Add(uint64(copiedIn))
		txnExecutionCancel()
	}
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
