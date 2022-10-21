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
	Db, err = sql.Open("postgres", psqlInfo)
	if err != nil {
		panic(err)
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
		panic(err)
	}
}

// ErrorHandling logs and handles postgresql errors
func ErrorHandling(sqlStatement string, err error, isCritical bool) {

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
