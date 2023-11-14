package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"go.uber.org/zap"
	"sync"
	"time"
)

type Connection struct {
	db *sql.DB
}

var conn *Connection
var once sync.Once

func Init() *Connection {
	once.Do(func() {
		zap.S().Debugf("Setting up postgresql")
		// Postgres
		PQHost, err := env.GetAsString("POSTGRES_HOST", false, "db")
		if err != nil {
			zap.S().Fatalf("Failed to get POSTGRES_HOST from env")
		}
		PQPort, err := env.GetAsInt("POSTGRES_PORT", false, 5432)
		if err != nil {
			zap.S().Fatalf("Failed to get POSTGRES_PORT from env")
		}
		PQUser, err := env.GetAsString("POSTGRES_USER", true, "")
		if err != nil {
			zap.S().Fatalf("Failed to get POSTGRES_USER from env")
		}
		PQPassword, err := env.GetAsString("POSTGRES_PASSWORD", true, "")
		if err != nil {
			zap.S().Fatalf("Failed to get POSTGRES_PASSWORD from env")
		}
		PQDBName, err := env.GetAsString("POSTGRES_DATABASE", true, "")
		if err != nil {
			zap.S().Fatalf("Failed to get POSTGRES_DATABASE from env")
		}
		PQSSLMode, err := env.GetAsString("POSTGRES_SSL_MODE", false, "require")
		if err != nil {
			zap.S().Fatalf("Failed to get POSTGRES_SSL_MODE from env")
		}

		conString := fmt.Sprintf("host=%s port=%d user =%s password=%s dbname=%s sslmode=%s", PQHost, PQPort, PQUser, PQPassword, PQDBName, PQSSLMode)

		var db *sql.DB
		db, err = sql.Open("postgres", conString)
		if err != nil {
			zap.S().Fatalf("Failed to open connection to postgres database: %s", err)
		}

		conn = &Connection{
			db: db,
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

func GetHealthCheck() healthcheck.Check {
	return func() error {
		if Init().IsAvailable() {
			return nil
		} else {
			return errors.New("Healthcheck failed to reach database")
		}
	}
}
