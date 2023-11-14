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

type DBValueNumeric struct {
	Timestamp int64
	Name      string
	Origin    string
	AssetId   int
	Value     int64
}
type DBValueString struct {
	Timestamp int64
	Name      string
	Origin    string
	AssetId   int
	Value     string
}

type Connection struct {
	db                     *sql.DB
	cache                  *lru.ARCCache
	numericalValuesChannel chan DBValueNumeric
	stringValuesChannel    chan DBValueString
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
			numericalValuesChannel: make(chan DBValueNumeric, 500_000),
			stringValuesChannel:    make(chan DBValueString, 500_000),
		}
		if !conn.IsAvailable() {
			zap.S().Fatalf("Database is not available !")
		}

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
		zap.S().Debug("Failed to ping database: %s", err)
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
		cacheKey.WriteRune('*') // This char cannot occure in the topic, and therefore can be safely used as seperator
		cacheKey.WriteRune('s')
		cacheKey.WriteString(*topic.Site)
	}
	if topic.Area != nil {
		cacheKey.WriteRune('*') // This char cannot occure in the topic, and therefore can be safely used as seperator
		cacheKey.WriteRune('a')
		cacheKey.WriteString(*topic.Area)
	}
	if topic.ProductionLine != nil {
		cacheKey.WriteRune('*') // This char cannot occure in the topic, and therefore can be safely used as seperator
		cacheKey.WriteRune('p')
		cacheKey.WriteString(*topic.ProductionLine)
	}
	if topic.WorkCell != nil {
		cacheKey.WriteRune('*') // This char cannot occure in the topic, and therefore can be safely used as seperator
		cacheKey.WriteRune('w')
		cacheKey.WriteString(*topic.WorkCell)
	}
	if topic.OriginId != nil {
		cacheKey.WriteRune('*') // This char cannot occure in the topic, and therefore can be safely used as seperator
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

func (c *Connection) InsertHistorianNumericValue(value int64, timestampMs int64, origin string, topic *sharedStructs.TopicDetails, name string) error {
	assetId, err := c.GetOrInsertAsset(topic)
	if err != nil {
		return err
	}
	c.numericalValuesChannel <- DBValueNumeric{
		Timestamp: timestampMs,
		Name:      name,
		Origin:    origin,
		AssetId:   assetId,
		Value:     value,
	}
	return nil
}

func (c *Connection) InsertHistorianStringValue(value string, timestampMs int64, origin string, topic *sharedStructs.TopicDetails, name string) error {
	assetId, err := c.GetOrInsertAsset(topic)
	if err != nil {
		return err
	}
	c.stringValuesChannel <- DBValueString{
		Timestamp: timestampMs,
		Name:      name,
		Origin:    origin,
		AssetId:   assetId,
		Value:     value,
	}
	return nil
}

func (c *Connection) numericalWorker() {
	zap.S().Debugf("Starting numericalWorker")
	// The context is only used for preperation, not execution!
	preparationCtx, preparationCancel := context.WithTimeout(context.Background(), time.Second*30)
	defer preparationCancel()
	var err error

	var statementCreateTmpTag *sql.Stmt
	statementCreateTmpTag, err = c.db.PrepareContext(preparationCtx, `
CREATE TEMP TABLE tmp_tag
       ( LIKE tag INCLUDING DEFAULTS )
       ON COMMIT DROP;
`)
	if err != nil {
		zap.S().Fatalf("Failed to prepare statement for statementCreateTmpTag")
	}

	var statementCopyTable *sql.Stmt
	statementCopyTable, err = c.db.PrepareContext(preparationCtx, pq.CopyIn("tmp_tag", "timestamp", "name", "origin", "asset", "value"))

	if err != nil {
		zap.S().Fatalf("Failed to prepare statement for statementCopyTable")
	}

	var statementInsertSelect *sql.Stmt
	statementInsertSelect, err = c.db.PrepareContext(preparationCtx, `
	INSERT INTO tag (SELECT * FROM tmp_tag) ON CONFLICT DO NOTHING;
`)

	if err != nil {
		zap.S().Fatalf("Failed to prepare statement for statementCopyTable")
	}

	retries := int64(0)
	tickerEvery30Seconds := time.NewTicker(30 * time.Second)
	for {
		internal.SleepBackedOff(retries, time.Millisecond, time.Minute)
		// Create transaction
		txn, err := c.db.Begin()
		if err != nil {
			zap.S().Errorf("Failed to create transaction: %s", err)
			retries++
			continue
		}
		// Create the temp table for COPY
		stmt := txn.Stmt(statementCreateTmpTag)
		_, err = stmt.Exec()
		if err != nil {
			zap.S().Errorf("Failed to execute statementCreateTmpTag: %s", err)
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s", err)
			}
			retries++
			continue
		}

		// Create COPY
		stmtCopy := txn.Stmt(statementCopyTable)

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
					continue
				}
				shouldInsert = false
			case msg := <-c.numericalValuesChannel:
				_, err := stmtCopy.Exec(msg.Timestamp, msg.Name, msg.Origin, msg.AssetId, msg.Value)
				if err != nil {
					zap.S().Errorf("Failed to copy into tmp_tag: %s", err)
					shouldInsert = false
					continue
				}
				inserted++
				if inserted > 10000 {
					shouldInsert = false
				}
			}
		}

		// Cleanup the statement, dropping allocated memory
		err = stmtCopy.Close()
		if err != nil {
			zap.S().Warnf("Failed to close stmtCopy")
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s", err)
			}
			retries++
			continue
		}

		// Do insert via statementInsertSelect
		var stmtCopyToTag *sql.Stmt
		stmtCopyToTag = txn.Stmt(statementInsertSelect)

		if err != nil {
			zap.S().Warnf("Failed to prepare stmtCopyToTag")
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s", err)
			}
			retries++
			continue
		}

		_, err = stmtCopyToTag.Exec()

		if err != nil {
			zap.S().Warnf("Failed to execute stmtCopyToTag")
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s", err)
			}
			retries++
			continue
		}

		err = stmtCopyToTag.Close()

		if err != nil {
			zap.S().Warnf("Failed to close stmtCopyToTag")
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s", err)
			}
			retries++
			continue
		}

		err = txn.Commit()

		if err != nil {
			zap.S().Errorf("Failed to rollback transaction: %s", err)
			retries++
			continue
		}
		retries = 0
	}
}

func (c *Connection) stringWorker() {
	zap.S().Debugf("Starting stringWorker")
	// The context is only used for preperation, not execution!
	preparationCtx, preparationCancel := context.WithTimeout(context.Background(), time.Second*30)
	defer preparationCancel()
	var err error

	var statementCreateTmpTagString *sql.Stmt
	statementCreateTmpTagString, err = c.db.PrepareContext(preparationCtx, `
CREATE TEMP TABLE tmp_tag_string
       ( LIKE tag_string INCLUDING DEFAULTS )
       ON COMMIT DROP;
`)
	if err != nil {
		zap.S().Fatalf("Failed to prepare statement for statementCreateTmpTagString")
	}

	var statementCopyTable *sql.Stmt
	statementCopyTable, err = c.db.PrepareContext(preparationCtx, pq.CopyIn("tmp_tag_string", "timestamp", "name", "origin", "asset", "value"))

	if err != nil {
		zap.S().Fatalf("Failed to prepare statement for statementCopyTable")
	}

	var statementInsertSelect *sql.Stmt
	statementInsertSelect, err = c.db.PrepareContext(preparationCtx, `
	INSERT INTO tag_string (SELECT * FROM tmp_tag_string) ON CONFLICT DO NOTHING;
`)

	if err != nil {
		zap.S().Fatalf("Failed to prepare statement for statementCopyTable")
	}

	retries := int64(0)
	tickerEvery30Seconds := time.NewTicker(30 * time.Second)
	for {
		internal.SleepBackedOff(retries, time.Millisecond, time.Minute)
		// Create transaction
		txn, err := c.db.Begin()
		if err != nil {
			zap.S().Errorf("Failed to create transaction: %s", err)
			retries++
			continue
		}
		// Create the temp table for COPY
		stmt := txn.Stmt(statementCreateTmpTagString)
		_, err = stmt.Exec()
		if err != nil {
			zap.S().Errorf("Failed to execute statementCreateTmpTagString: %s", err)
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s", err)
			}
			retries++
			continue
		}

		// Create COPY
		stmtCopy := txn.Stmt(statementCopyTable)

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
					continue
				}
				shouldInsert = false
			case msg := <-c.numericalValuesChannel:
				_, err := stmtCopy.Exec(msg.Timestamp, msg.Name, msg.Origin, msg.AssetId, msg.Value)
				if err != nil {
					zap.S().Errorf("Failed to copy into tmp_tag_string: %s", err)
					shouldInsert = false
					continue
				}
				inserted++
				if inserted > 10000 {
					shouldInsert = false
				}
			}
		}

		// Cleanup the statement, dropping allocated memory
		err = stmtCopy.Close()
		if err != nil {
			zap.S().Warnf("Failed to close stmtCopy")
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s", err)
			}
			retries++
			continue
		}

		// Do insert via statementInsertSelect
		var stmtCopyToTagString *sql.Stmt
		stmtCopyToTagString = txn.Stmt(statementInsertSelect)

		if err != nil {
			zap.S().Warnf("Failed to prepare stmtCopyToTagString")
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s", err)
			}
			retries++
			continue
		}

		_, err = stmtCopyToTagString.Exec()

		if err != nil {
			zap.S().Warnf("Failed to execute stmtCopyToTagString")
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s", err)
			}
			retries++
			continue
		}

		err = stmtCopyToTagString.Close()

		if err != nil {
			zap.S().Warnf("Failed to close stmtCopyToTagString")
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s", err)
			}
			retries++
			continue
		}

		err = txn.Commit()

		if err != nil {
			zap.S().Errorf("Failed to rollback transaction: %s", err)
			retries++
			continue
		}
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
