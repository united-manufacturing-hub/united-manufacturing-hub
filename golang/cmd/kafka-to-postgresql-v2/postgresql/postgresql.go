package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	lru "github.com/hashicorp/golang-lru"
	"github.com/heptiolabs/healthcheck"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"github.com/united-manufacturing-hub/umh-utils/env"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
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

		zap.S().Infof("Connecting to %s@%s:%d/%s [%s]", PQUser, PQHost, PQPort, PQDBName, PQSSLMode)

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
		tablesToCheck := []string{"asset", "tag", "tag_string"}
		for _, table := range tablesToCheck {
			var tableName string
			query := `SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1`
			row := db.QueryRow(query, table)
			err := row.Scan(&tableName)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					zap.S().Fatalf("Table %s does not exist in the database", table)
				} else {
					zap.S().Fatalf("Failed to check for table %s: %s", table, err)
				}
			}
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
		return value.(int), nil
	}

	// Prepare an upsert query with RETURNING clause
	// This is atomic
	upsertQuery := `INSERT INTO asset (enterprise, site, area, line, workcell, origin_id) 
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
	selectQuery := `SELECT id FROM asset WHERE enterprise = $1 AND 
		(site IS NULL OR site = $2) AND 
		(area IS NULL OR area = $3) AND 
		(line IS NULL OR line = $4) AND 
		(workcell IS NULL OR workcell = $5) AND 
		(origin_id IS NULL OR origin_id = $6)`

	err = c.db.QueryRow(selectQuery, topic.Enterprise, topic.Site, topic.Area, topic.ProductionLine, topic.WorkCell, topic.OriginId).Scan(&id)
	if err != nil {
		return 0, err
	}

	c.cache.Add(cacheKey.String(), id)

	return id, nil
}

func (c *Connection) InsertHistorianValue(value *sharedStructs.Value, timestampMs int64, origin string, topic *sharedStructs.TopicDetails, name string) error {
	assetId, err := c.GetOrInsertAsset(topic)
	if err != nil {
		return err
	}
	if value.IsNumeric {
		c.numericalValuesChannel <- DBValue{
			Timestamp: timestampMs,
			Name:      name,
			Origin:    origin,
			AssetId:   assetId,
			Value:     value,
		}
	} else {
		c.stringValuesChannel <- DBValue{
			Timestamp: timestampMs,
			Name:      name,
			Origin:    origin,
			AssetId:   assetId,
			Value:     value,
		}
	}
	zap.S().Debugf("Inserted value. NumericChannelLenght: %d/%d, StringChannelLenght: %d/%d", len(c.numericalValuesChannel), cap(c.numericalValuesChannel), len(c.stringValuesChannel), cap(c.stringValuesChannel))
	return nil
}

func (c *Connection) tagWorker(tableName string, channel chan DBValue) {
	zap.S().Debugf("Starting tagWorker for %s", tableName)
	// The context is only used for preparation, not execution!
	preparationCtx, preparationCancel := context.WithTimeout(context.Background(), time.Second*30)
	defer preparationCancel()
	var err error

	tableNameTemp := fmt.Sprintf("tmp_%s", tableName)

	var statementCreateTmpTag *sql.Stmt
	statementCreateTmpTag, err = c.db.PrepareContext(preparationCtx, fmt.Sprintf(`
CREATE TEMP TABLE %s
       ( LIKE %s INCLUDING DEFAULTS )
       ON COMMIT DROP;
`, tableNameTemp, tableName)) // This is safe, as tableName is not user provided
	if err != nil {
		zap.S().Fatalf("Failed to prepare statement for statementCreateTmpTag: %v (%s)", err, tableName)
	}

	tickerEvery30Seconds := time.NewTicker(5 * time.Second)
	for {
		time.Sleep(1 * time.Second)
		txnExecutionCtx, txnExecutionCancel := context.WithTimeout(context.Background(), time.Minute)
		// Create transaction
		var txn *sql.Tx
		txn, err = c.db.BeginTx(txnExecutionCtx, nil)
		if err != nil {
			zap.S().Errorf("Failed to create transaction: %s (%s)", err, tableName)

			txnExecutionCancel()
			continue
		}
		// Create the temp table for COPY
		stmt := txn.Stmt(statementCreateTmpTag)
		_, err = stmt.ExecContext(txnExecutionCtx)
		if err != nil {
			zap.S().Errorf("Failed to execute statementCreateTmpTag: %s (%s)", err, tableName)
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			}

			txnExecutionCancel()
			continue
		}
		txnPrepareContextCtx, txnPrepareContextCancel := context.WithTimeout(context.Background(), time.Second*5)

		var statementCopyTable *sql.Stmt
		statementCopyTable, err = txn.PrepareContext(txnPrepareContextCtx, pq.CopyIn(tableNameTemp, "timestamp", "name", "origin", "asset_id", "value"))
		txnPrepareContextCancel()

		if err != nil {
			zap.S().Errorf("Failed to execute statementCreateTmpTag: %s (%s)", err, tableName)
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			}

			txnExecutionCancel()
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
				zap.S().Debugf("Got tick, manually committing")
				if inserted == 0 {
					zap.S().Debugf("Skipping")
				}
				shouldInsert = false
			case msg := <-channel:
				zap.S().Debugf("Got a message to insert")

				if tableName == "tag" {
					_, err = statementCopyTable.ExecContext(txnExecutionCtx, msg.Timestamp, msg.Name, msg.Origin, msg.AssetId, msg.GetValue().(float64))
				} else {
					_, err = statementCopyTable.ExecContext(txnExecutionCtx, msg.Timestamp, msg.Name, msg.Origin, msg.AssetId, msg.GetValue().(string))
				}
				if err != nil {
					zap.S().Errorf("Failed to copy into %s: %s (%s)", tableNameTemp, err, tableName)
					shouldInsert = false
					continue
				}
				inserted++
				if inserted > 10000 {
					zap.S().Debugf("Got 10k, manually committing")
					shouldInsert = false
				}
			}
		}

		zap.S().Debugf("PREP INSERT SELECT")
		txnPrepareContextCtx, txnPrepareContextCancel = context.WithTimeout(context.Background(), time.Second*5)
		var statementInsertSelect *sql.Stmt
		statementInsertSelect, err = txn.PrepareContext(txnPrepareContextCtx, fmt.Sprintf(`
	INSERT INTO %s (SELECT * FROM %s) ON CONFLICT DO NOTHING;
`, tableName, tableNameTemp)) // This is safe, as tableName is not user provided
		txnPrepareContextCancel()

		if err != nil {
			zap.S().Warnf("Failed to prepare statementInsertSelect: %s (%s)", err, tableName)
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			}
			txnExecutionCancel()
			continue
		}

		zap.S().Debugf("EXEC INSERT SELECT")
		// Do insert via statementInsertSelect
		stmtCopyToTag := txn.Stmt(statementInsertSelect)
		_, err = stmtCopyToTag.ExecContext(txnExecutionCtx)

		if err != nil {
			zap.S().Warnf("Failed to execute stmtCopyToTag: %s (%s)", err, tableName)
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			}
			txnExecutionCancel()
			continue
		}

		zap.S().Debugf("CLOSE COPY TO TMP")
		err = stmtCopyToTag.Close()
		if err != nil {
			zap.S().Warnf("Failed to close stmtCopyToTag: %s (%s)", err, tableName)
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			}
			txnExecutionCancel()
			continue
		}

		zap.S().Debugf("CLOSE COPY TABLE")
		// Cleanup the statement, dropping allocated memory
		err = statementCopyTable.Close()
		if err != nil {
			zap.S().Warnf("Failed to close stmtCopy: %s (%s)", err, tableName)
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			}
			txnExecutionCancel()
			continue
		}

		zap.S().Debugf("Pre-commit")
		now := time.Now()
		err = txn.Commit()
		zap.S().Debugf("Committing to postgresql took: %s", time.Since(now))

		if err != nil {
			zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			txnExecutionCancel()
			continue
		}
		zap.S().Infof("Inserted %d values inside the %s table", inserted, tableName)
		txnExecutionCancel()
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
