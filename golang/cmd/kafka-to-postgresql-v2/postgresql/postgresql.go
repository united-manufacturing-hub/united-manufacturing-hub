package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	lru "github.com/hashicorp/golang-lru"
	"github.com/heptiolabs/healthcheck"
	"github.com/lib/pq"
	"github.com/united-manufacturing-hub/umh-utils/env"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"strings"
	"sync"
	"time"
)

type DBValue struct {
	Timestamp int64
	Name      string
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
	db                     *sql.DB
	cache                  *lru.ARCCache
	numericalValuesChannel chan DBValue
	stringValuesChannel    chan DBValue
}

var conn *Connection
var once sync.Once

func Init() *Connection {
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

		conString := fmt.Sprintf("host=%s port=%d user =%s password=%s dbname=%s sslmode=%s", PQHost, PQPort, PQUser, PQPassword, PQDBName, PQSSLMode)

		var db *sql.DB
		db, err = sql.Open("postgres", conString)
		if err != nil {
			zap.S().Fatalf("Failed to open connection to postgres database: %s", err)
		}

		PQLRUSize, err := env.GetAsInt("POSTGRES_LRU_CACHE_SIZE", false, 1000)
		if err != nil {
			zap.S().Fatalf("Failed to get POSTGRES_LRU_CACHE_SIZE from env: %s", err)
		}
		cache, err := lru.NewARC(PQLRUSize)
		conn = &Connection{
			db:                     db,
			cache:                  cache,
			numericalValuesChannel: make(chan DBValue, 500_000),
			stringValuesChannel:    make(chan DBValue, 500_000),
		}
		if !conn.IsAvailable() {
			zap.S().Fatalf("Database is not available !")
		}
		go conn.tagWorker("tag", conn.numericalValuesChannel)
		go conn.tagWorker("tag_string", conn.stringValuesChannel)

	})
	return conn
}

func (c *Connection) IsAvailable() bool {
	if c.db == nil {
		return false
	}
	ctx, cncl := context.WithTimeout(context.Background(), time.Second*5)
	defer cncl()
	err := c.db.PingContext(ctx)
	if err != nil {
		zap.S().Debugf("Failed to ping database: %s", err)
		return false
	}
	return true
}

func (c *Connection) GetOrInsertAsset(topic *sharedStructs.TopicDetails) (int, error) {
	if c.db == nil {
		return 0, errors.New("database is nil")
	}
	// Attempt cache lookup
	var cacheKey strings.Builder
	cacheKey.WriteString(topic.Enterprise)
	if topic.Site != nil {
		cacheKey.WriteRune('*') // This char cannot occur in the topic, and therefore can be safely used as a seperator
		cacheKey.WriteRune('s')
		cacheKey.WriteString(*topic.Site)
	}
	if topic.Area != nil {
		cacheKey.WriteRune('*') // This char cannot occur in the topic, and therefore can be safely used as a seperator
		cacheKey.WriteRune('a')
		cacheKey.WriteString(*topic.Area)
	}
	if topic.ProductionLine != nil {
		cacheKey.WriteRune('*') // This char cannot occur in the topic, and therefore can be safely used as a seperator
		cacheKey.WriteRune('p')
		cacheKey.WriteString(*topic.ProductionLine)
	}
	if topic.WorkCell != nil {
		cacheKey.WriteRune('*') // This char cannot occur in the topic, and therefore can be safely used as a seperator
		cacheKey.WriteRune('w')
		cacheKey.WriteString(*topic.WorkCell)
	}
	if topic.OriginId != nil {
		cacheKey.WriteRune('*') // This char cannot occur in the topic, and therefore can be safely used as a seperator
		cacheKey.WriteRune('o')
		cacheKey.WriteString(*topic.OriginId)
	}

	value, ok := c.cache.Get(cacheKey)
	if ok {
		return value.(int), nil
	}

	// Prepare an upsert query with RETURNING clause
	// This is atomic
	upsertQuery := `INSERT INTO assets (enterprise, site, area, line, workcell, origin_id) 
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (enterprise, site, area, line, workcell, origin_id) 
		DO UPDATE SET enterprise = EXCLUDED.enterprise
		RETURNING id`

	var id int
	err := c.db.QueryRow(upsertQuery, topic.Enterprise, topic.Site, topic.Area, topic.ProductionLine, topic.WorkCell, topic.OriginId).Scan(&id)
	if err == nil {
		// Asset was inserted or updated and id was retrieved
		return id, nil
	}

	// If the upsert didn't return an id, retrieve it with a select query
	selectQuery := `SELECT id FROM assets WHERE enterprise = $1 AND 
		(site IS NULL OR site = $2) AND 
		(area IS NULL OR area = $3) AND 
		(line IS NULL OR line = $4) AND 
		(workcell IS NULL OR workcell = $5) AND 
		(origin_id IS NULL OR origin_id = $6)`

	err = c.db.QueryRow(selectQuery, topic.Enterprise, topic.Site, topic.Area, topic.ProductionLine, topic.WorkCell, topic.OriginId).Scan(&id)
	if err != nil {
		return 0, err
	}

	c.cache.Add(cacheKey, id)

	return id, nil
}

func (c *Connection) InsertHistorianValue(value *sharedStructs.Value, timestampMs int64, origin string, topic *sharedStructs.TopicDetails, name string) error {
	assetId, err := c.GetOrInsertAsset(topic)
	if err != nil {
		return err
	}
	c.numericalValuesChannel <- DBValue{
		Timestamp: timestampMs,
		Name:      name,
		Origin:    origin,
		AssetId:   assetId,
		Value:     value,
	}
	return nil
}

func (c *Connection) tagWorker(tableName string, channel chan DBValue) {
	zap.S().Debugf("Starting tagWorker for %s", tableName)
	// The context is only used for preparation, not execution!
	preparationCtx, preparationCancel := context.WithTimeout(context.Background(), time.Second*30)
	defer preparationCancel()
	var err error

	var statementCreateTmpTag *sql.Stmt
	statementCreateTmpTag, err = c.db.PrepareContext(preparationCtx, fmt.Sprintf(`
CREATE TEMP TABLE tmp_%s
       ( LIKE tag INCLUDING DEFAULTS )
       ON COMMIT DROP;
`, tableName))
	if err != nil {
		zap.S().Fatalf("Failed to prepare statement for statementCreateTmpTag: %v (%s)", err, tableName)
	}

	retries := int64(0)
	tickerEvery30Seconds := time.NewTicker(5 * time.Second)
	for {
		internal.SleepBackedOff(retries, time.Millisecond, time.Minute)
		// Create transaction
		var txn *sql.Tx
		txn, err = c.db.Begin()
		if err != nil {
			zap.S().Errorf("Failed to create transaction: %s (%s)", err, tableName)
			retries++
			continue
		}
		// Create the temp table for COPY
		stmt := txn.Stmt(statementCreateTmpTag)
		_, err = stmt.Exec()
		if err != nil {
			zap.S().Errorf("Failed to execute statementCreateTmpTag: %s (%s)", err, tableName)
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			}
			retries++
			continue
		}

		var statementCopyTable *sql.Stmt
		statementCopyTable, err = txn.Prepare(pq.CopyIn(fmt.Sprintf("tmp_%s", tableName), "timestamp", "name", "origin", "asset", "value"))

		if err != nil {
			zap.S().Errorf("Failed to execute statementCreateTmpTag: %s (%s)", err, tableName)
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			}
			retries++
			continue
		}

		// Copy in data, until:
		// 30-second Ticker
		// 10000 entries
		// then commit

		inserted := 0
		shouldInsert := true
		for shouldInsert {
			select {
			case <-tickerEvery30Seconds.C:
				if inserted == 0 {
					// Nothing to do, let's just wait longer
					zap.S().Debugf("Nothing to insert")
					continue
				}
				shouldInsert = false
			case msg := <-channel:
				_, err = statementCopyTable.Exec(msg.Timestamp, msg.Name, msg.Origin, msg.AssetId, msg.GetValue())
				if err != nil {
					zap.S().Errorf("Failed to copy into tmp_tag: %s (%s)", err, tableName)
					shouldInsert = false
					continue
				}
				inserted++
				if inserted > 10000 {
					shouldInsert = false
				}
			}
		}

		var statementInsertSelect *sql.Stmt
		statementInsertSelect, err = c.db.Prepare(fmt.Sprintf(`
	INSERT INTO %s (SELECT * FROM tmp_%s) ON CONFLICT DO NOTHING;
`, tableName, tableName))

		if err != nil {
			zap.S().Warnf("Failed to prepare statementInsertSelect: %s (%s)", err, tableName)
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			}
			retries++
			continue
		}

		// Do insert via statementInsertSelect
		stmtCopyToTag := txn.Stmt(statementInsertSelect)
		_, err = stmtCopyToTag.Exec()

		if err != nil {
			zap.S().Warnf("Failed to execute stmtCopyToTag: %s (%s)", err, tableName)
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			}
			retries++
			continue
		}

		err = stmtCopyToTag.Close()

		if err != nil {
			zap.S().Warnf("Failed to close stmtCopyToTag: %s (%s)", err, tableName)
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			}
			retries++
			continue
		}

		// Cleanup the statement, dropping allocated memory
		err = statementCopyTable.Close()
		if err != nil {
			zap.S().Warnf("Failed to close stmtCopy: %s (%s)", err, tableName)
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			}
			retries++
			continue
		}

		err = txn.Commit()

		if err != nil {
			zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			retries++
			continue
		}
		zap.S().Infof("Inserted %d values inside the %s table", inserted, tableName)
		retries = 0
	}
}

func GetHealthCheck() healthcheck.Check {
	return func() error {
		if Init().IsAvailable() {
			return nil
		} else {
			return errors.New("healthcheck failed to reach database")
		}
	}
}
