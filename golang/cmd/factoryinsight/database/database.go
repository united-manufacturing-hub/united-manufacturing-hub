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
	"database/sql"
	"fmt"
	"github.com/EagleChen/mapmutex"
	"github.com/omeid/pgerror"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"syscall"
)

var (
	Db                      *sql.DB
	Mutex                   *mapmutex.Mutex
	GracefulShutdownChannel = make(chan os.Signal, 1)
)

// Connect setups the Db and stores the handler in a global variable in database.go
func Connect(
	PQUser string,
	PQPassword string,
	PWDBName string,
	PQHost string,
	PQPort int,
	gracefulShutdownChannel chan os.Signal) {

	GracefulShutdownChannel = gracefulShutdownChannel

	psqlInfo := fmt.Sprintf(
		"host=%s port=%d user=%s "+"password=%s dbname=%s sslmode=require",
		PQHost,
		PQPort,
		PQUser,
		PQPassword,
		PWDBName)
	var err error
	Db, err = sql.Open("postgres", psqlInfo)
	if err != nil {
		zap.S().Fatalf("Failed to open database: %s", err)
	}

	Mutex = mapmutex.NewCustomizedMapMutex(
		800,
		100000000,
		10,
		1.1,
		0.2) // default configs: maxDelay:  100000000, // 0.1 second baseDelay: 10,        // 10 nanosecond
}

// Shutdown closes all database connections
func Shutdown() {
	if err := Db.Close(); err != nil {
		zap.S().Fatalf("Failed to close database: %s", err)
	}
}

// ErrorHandling logs and handles postgresql errors
func ErrorHandling(sqlStatement string, err error, isCritical bool) {
	zap.S().Debugf("ErrorHandling: sqlStatement: %s, err: %s, isCritical: %t", sqlStatement, err, isCritical)
	if e := pgerror.ConnectionException(err); e != nil {
		zap.S().Errorw(
			"PostgreSQL failed: ConnectionException",
			"error", err,
			"sqlStatement", sqlStatement,
		)
		isCritical = true
	} else {
		zap.S().Errorw(
			"PostgreSQL failed. ",
			"error", err,
			"sqlStatement", sqlStatement,
		)
	}

	if isCritical {
		signal.Notify(GracefulShutdownChannel, syscall.SIGTERM)
	}
}
