package postgresql

import (
	"context"
	"database/sql"
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
		conn = &Connection{
			db:                     db,
			cache:                  cache,
			numericalValuesChannel: make(chan DBValue, 500_000),
			stringValuesChannel:    make(chan DBValue, 500_000),
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
				if errors.Is(err, sql.ErrNoRows) {
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
	lastNumericalReceived := uint64(0)
	lastStringsReceived := uint64(0)
	lastDatabaseInserted := uint64(0)

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

		// Calculating rates per second for the past 10 seconds
		numericalRate := float64(currentNumericalReceived-lastNumericalReceived) / 10.0
		stringRate := float64(currentStringsReceived-lastStringsReceived) / 10.0
		databaseInsertionRate := float64(currentDatabaseInserted-lastDatabaseInserted) / 10.0

		// Logging the stats
		zap.S().Infof("LRU Hit Percentage: %.2f%%, Numerical Entries/s: %.2f, String Entries/s: %.2f, DB Insertions/s: %.2f",
			lruHitPercentage, numericalRate, stringRate, databaseInsertionRate)

		// Update the last values for the next tick
		lastNumericalReceived = currentNumericalReceived
		lastStringsReceived = currentStringsReceived
		lastDatabaseInserted = currentDatabaseInserted

		// Check if there were no database insertions
		if currentDatabaseInserted == 0 {
			zap.S().Info("No database insertions so far")
		}
	}
}

func (c *Connection) GetOrInsertAsset(topic *sharedStructs.TopicDetails) (int, error) {
	if c.db == nil {
		return 0, errors.New("database is nil")
	}
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

	value, ok := c.cache.Get(cacheKey.String())
	if ok {
		c.lruHits.Add(1)
		return value.(int), nil
	}
	c.lruMisses.Add(1)

	// Prepare an upsert query with RETURNING clause
	// This is atomic
	upsertQuery := `INSERT INTO asset (enterprise, site, area, line, workcell, origin_id) 
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (enterprise, site, area, line, workcell, origin_id) 
		DO UPDATE SET enterprise = EXCLUDED.enterprise
		RETURNING id`

	queryRowContext, queryRowContextCncl := get5SecondContext()
	defer queryRowContextCncl()
	var id int
	err := c.db.QueryRow(queryRowContext, upsertQuery, topic.Enterprise, topic.Site, topic.Area, topic.ProductionLine, topic.WorkCell, topic.OriginId).Scan(&id)
	if err == nil {
		// Asset was inserted or updated and id was retrieved
		return id, nil
	}

	// If the upsert didn't return an id, retrieve it with a select query
	selectQuery := `SELECT id FROM asset WHERE enterprise = $1 AND 
		(site IS NULL OR site = $2) AND 
		(area IS NULL OR area = $3) AND 
		(line IS NULL OR line = $4) AND 
		(workcell IS NULL OR workcell = $5) AND 
		(origin_id IS NULL OR origin_id = $6)`

	selectRowContext, selectRowContextCncl := get5SecondContext()
	defer selectRowContextCncl()
	err = c.db.QueryRow(selectRowContext, selectQuery, topic.Enterprise, topic.Site, topic.Area, topic.ProductionLine, topic.WorkCell, topic.OriginId).Scan(&id)
	if err != nil {
		return 0, err
	}

	c.cache.Add(cacheKey.String(), id)

	return id, nil
}

func (c *Connection) InsertHistorianValue(value *sharedStructs.Value, timestampMs int64, origin string, topic *sharedStructs.TopicDetails) error {
	assetId, err := c.GetOrInsertAsset(topic)
	if err != nil {
		return err
	}
	seconds := timestampMs / 1000
	nanoseconds := (timestampMs % 1000) * 1000000
	timestamp := time.Unix(seconds, nanoseconds)
	if value.IsNumeric {
		c.numericalValuesChannel <- DBValue{
			Timestamp: timestamp,
			Origin:    origin,
			AssetId:   assetId,
			Value:     value,
		}
		c.numericalReceived.Add(1)
	} else {
		c.stringValuesChannel <- DBValue{
			Timestamp: timestamp,
			Origin:    origin,
			AssetId:   assetId,
			Value:     value,
		}
		c.stringsReceived.Add(1)
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
	for {
		time.Sleep(1 * time.Second)
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
		var copiedIn int64
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
		zap.S().Debugf("Committing to postgresql took: %s", time.Since(now))

		if err != nil {
			zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			txnExecutionCancel()
			continue
		}
		zap.S().Infof("Inserted %d values inside the %s table", copiedIn, tableName)
		c.databaseInserted.Add(1)
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
