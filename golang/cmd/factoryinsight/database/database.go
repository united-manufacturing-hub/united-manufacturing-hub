// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package database

import (
	"context"
	"fmt"
	"github.com/EagleChen/mapmutex"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/omeid/pgerror"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

var (
	DBConnPool              *pgxpool.Pool
	Mutex                   *mapmutex.Mutex
	GracefulShutdownChannel = make(chan os.Signal, 1)
)

// Connect setups the DBConnPool and stores the handler in a global variable in database.go
func Connect(PQUser string, PQPassword string, PWDBName string, PQHost string, PQPort int, gracefulShutdownChannel chan os.Signal) {
	GracefulShutdownChannel = gracefulShutdownChannel

	psqlInfo := fmt.Sprintf(
		"host=%s port=%d user=%s "+"password=%s dbname=%s sslmode=require",
		PQHost,
		PQPort,
		PQUser,
		PQPassword,
		PWDBName)
	var err error

	parseConfig, err := pgxpool.ParseConfig(psqlInfo)
	if err != nil {
		zap.S().Fatalf("Failed to parse config: %s", err)
	}

	parseConfig.MinConns = int32(runtime.NumCPU())
	if parseConfig.MinConns < 4 {
		parseConfig.MinConns = 4
	}
	parseConfig.MaxConnIdleTime = 5 * time.Minute
	parseConfig.MaxConnLifetime = 10 * time.Minute

	parseConfig.BeforeConnect = func(ctx context.Context, conn *pgx.ConnConfig) error {
		zap.S().Debugf("BeforeConnect: ctx: %s, conn: %s", ctx, conn)
		return nil
	}

	parseConfig.BeforeClose = func(conn *pgx.Conn) {
		zap.S().Debugf("BeforeClose: conn: %v", conn)
	}

	connCtx, conncnc := context.WithTimeout(context.Background(), 5*time.Second)
	defer conncnc()
	DBConnPool, err = pgxpool.NewWithConfig(connCtx, parseConfig)
	if err != nil {
		zap.S().Fatalf("Failed to open database: %s", err)
	}
	go pingDB()

	Mutex = mapmutex.NewCustomizedMapMutex(
		800,
		100000000,
		10,
		1.1,
		0.2) // default configs: maxDelay:  100000000, // 0.1 second baseDelay: 10,        // 10 nanosecond
}

func pingDB() {
	for {
		err := DBConnPool.Ping(context.Background())
		if err != nil {
			zap.S().Errorf("Failed to ping database: %s", err)
		}
		time.Sleep(5 * time.Second)
	}
}

// Shutdown closes all database connections
func Shutdown() {
	DBConnPool.Close()
}

// ErrorHandling logs and handles postgresql errors
func ErrorHandling(sqlStatement string, err error, isCritical bool) {
	zap.S().Debugf("ErrorHandling: sqlStatement: %s, err: %s, isCritical: %t", sqlStatement, err, isCritical)
	stackTrace := make([]byte, 1024*8)
	written := runtime.Stack(stackTrace, true)
	if e := pgerror.ConnectionException(err); e != nil {
		zap.S().Errorw(
			"PostgreSQL failed: ConnectionException",
			"error", err,
			"sqlStatement", sqlStatement,
			"stackTrace", string(stackTrace[:written]),
		)

		isCritical = true
	} else {
		zap.S().Errorw(
			"PostgreSQL failed.",
			"error", err,
			"sqlStatement", sqlStatement,
			"stackTrace", string(stackTrace[:written]),
		)
	}

	if isCritical {
		signal.Notify(GracefulShutdownChannel, syscall.SIGTERM)
	}
}

func Query(sql string, args ...any) (pgx.Rows, error) {
	return queryRepeatable(sql, 0, args...)
}
func QueryRow(sql string, args ...any) pgx.Row {
	return DBConnPool.QueryRow(context.Background(), sql, args...)
}

func queryRepeatable(sql string, count uint, args ...any) (pgx.Rows, error) {
	rows, err := DBConnPool.Query(context.Background(), sql, args...)
	if err != nil {
		if count > 10 {
			return nil, err
		}
		// Check if err is a connection error
		if pgerror.ConnectionException(err) != nil {
			zap.S().Debugf("Failed to connect to database: %s [retrying]", err)
			time.Sleep(time.Duration(count) * time.Millisecond * 100)
			return queryRepeatable(sql, count+1, args...)
		}
		return nil, err
	}
	return rows, nil
}
