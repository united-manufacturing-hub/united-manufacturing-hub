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

type DBRow struct {
	Timestamp time.Time
	Name      string
	Origin    string
	AssetId   int
	Value     any
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
	DatabaseInsertionRate10Seconds      float64
}

type Connection struct {
	db                *pgxpool.Pool
	cache             *lru.ARCCache
	numericalReceived atomic.Uint64
	stringsReceived   atomic.Uint64
	databaseInserted  atomic.Uint64
	lruHits           atomic.Uint64
	lruMisses         atomic.Uint64
	commits           atomic.Uint64
	commitTime        atomic.Uint64
	metrics           Metrics
	metricsLock       sync.RWMutex
	numericSource     chan DBRow
	stringSource      chan DBRow
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
		TableSize, err := env.GetAsInt("VALUE_TABLE_SIZE", false, 100000)
		if err != nil {
			zap.S().Fatalf("Failed to get VALUE_CHANNEL_SIZE from env: %s", err)
		}
		FlushInterval, err := env.GetAsInt("FLUSH_INTERVAL_SECONDS", false, 10)
		if err != nil {
			zap.S().Fatalf("Failed to get VALUE_CHANNEL_SIZE from env: %s", err)
		}
		conn = &Connection{
			db:            db,
			cache:         cache,
			stringSource:  make(chan DBRow, ChannelSize),
			numericSource: make(chan DBRow, ChannelSize),
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

		go conn.tagWorker("tag", conn.numericSource, TableSize, FlushInterval)
		go conn.tagWorker("tag_string", conn.stringSource, TableSize, FlushInterval)

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
	lastInserted := uint64(0)
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

		numericalChannelFill := len(c.numericSource)
		numericalChannelFillPercentage := float64(0)
		if numericalChannelFill > 0 {
			numericalChannelFillPercentage = float64(numericalChannelFill) / float64(cap(c.numericSource)) * 100
		}

		stringsChannelFill := len(c.stringSource)
		stringsChannelFillPercentage := float64(0)
		if stringsChannelFill > 0 {
			stringsChannelFillPercentage = float64(stringsChannelFill) / float64(cap(c.stringSource)) * 100
		}
		totalCommits := c.commits.Load()
		totalCommitTime := c.commitTime.Load() // Assuming this is in milliseconds

		averageCommitDuration := float64(0)
		if totalCommits > 0 {
			averageCommitDuration = float64(totalCommitTime) / float64(totalCommits)
		}
		// Inserted per second (avg 10 seconds)
		inserted := currentDatabaseInserted - lastInserted
		lastInserted = currentDatabaseInserted
		databaseInsertionRate10Sec := float64(inserted) / 10

		c.metricsLock.Lock()
		c.metrics = Metrics{
			LRUHitPercentage:                    lruHitPercentage,
			NumericalChannelFillPercentage:      numericalChannelFillPercentage,
			StringChannelFillPercentage:         stringsChannelFillPercentage,
			DatabaseInsertions:                  c.databaseInserted.Load(),
			DatabaseInsertionRate:               databaseInsertionRate,
			DatabaseInsertionRate10Seconds:      databaseInsertionRate10Sec,
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
	var metricsClone Metrics
	metricsClone = c.metrics
	c.metricsLock.RUnlock()
	return metricsClone
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
		if value.IsNumeric {
			c.numericSource <- DBRow{
				Timestamp: timestamp,
				Name:      value.Name,
				Origin:    origin,
				AssetId:   assetId,
				Value:     *value.NumericValue,
			}
			c.numericalReceived.Add(1)
		} else {
			c.stringSource <- DBRow{
				Timestamp: timestamp,
				Name:      value.Name,
				Origin:    origin,
				AssetId:   assetId,
				Value:     *value.StringValue,
			}
			c.stringsReceived.Add(1)
		}
	}

	return nil
}

func (c *Connection) tagWorker(tableName string, source <-chan DBRow, maxBeforeFlush int, flushInterval int) {
	zap.S().Debugf("Starting tagWorker for %s", tableName)
	// The context is only used for preparation, not execution!

	ticker1Second := time.NewTicker(time.Duration(flushInterval) * time.Second)
	shallFlush := make(chan bool, 1)

	rowsToInsert := make([]DBRow, 0, maxBeforeFlush)

	for {
		select {
		case <-ticker1Second.C:
			c.flush(rowsToInsert, tableName)
			rowsToInsert = rowsToInsert[:0]
		case <-shallFlush:
			c.flush(rowsToInsert, tableName)
			rowsToInsert = rowsToInsert[:0]
		default:
			// Add to insertion table
			if len(rowsToInsert) == maxBeforeFlush {
				zap.S().Debugf("Reached capacity, flushing")
				shallFlush <- true
				continue
			}
			// Try to get value from source
			select {
			case val := <-source:
				rowsToInsert = append(rowsToInsert, val)
			default:
				zap.S().Debugf("Nothing to do, waiting")
				// There is nothing to do, just wait a bit
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}

func (c *Connection) flush(rows []DBRow, tableName string) {
	if len(rows) == 0 {
		return
	}
	var copiedIn int64
	var err error

	tableNameTemp := fmt.Sprintf("tmp_%s", tableName)

	txnExecutionCtx, txnExecutionCancel := get1MinuteContext()
	// Create transaction
	var txn pgx.Tx
	txn, err = c.db.BeginTx(txnExecutionCtx, pgx.TxOptions{})
	if err != nil {
		zap.S().Errorf("Failed to create transaction: %s (%s)", err, tableName)
		txnExecutionCancel()
		return
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
		return
	}
	copiedIn, err = txn.CopyFrom(txnExecutionCtx, pgx.Identifier{tableNameTemp}, []string{
		"timestamp", "name", "origin", "asset_id", "value",
	}, pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
		return []any{
			rows[i].Timestamp,
			rows[i].Name,
			rows[i].Origin,
			rows[i].AssetId,
			rows[i].Value,
		}, nil
	}))

	if err != nil {
		zap.S().Warnf("Failed to execute stmtCopyToTag: %s (%s)", err, tableName)
		rollbackCtx, rollbackCtxCncl := get5SecondContext()
		err = txn.Rollback(rollbackCtx)
		rollbackCtxCncl()
		if err != nil {
			zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
		}
		txnExecutionCancel()
		return
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
		return
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
		return
	}
	zap.S().Debugf("Inserted %d values inside the %s table", copiedIn, tableName)
	c.databaseInserted.Add(uint64(copiedIn))
	txnExecutionCancel()
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
