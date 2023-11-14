package postgresql

import (
	"github.com/united-manufacturing-hub/umh-utils/env"
	"go.uber.org/zap"
)

type Connection struct {
}

func Init() error {
	zap.S().Debugf("Setting up postgresql")
	// Postgres
	PQHost, err := env.GetAsString("POSTGRES_HOST", false, "db")
	if err != nil {
		return err
	}
	PQPort, err := env.GetAsInt("POSTGRES_PORT", false, 5432)
	if err != nil {
		return err
	}
	PQUser, err := env.GetAsString("POSTGRES_USER", true, "")
	if err != nil {
		return err
	}
	PQPassword, err := env.GetAsString("POSTGRES_PASSWORD", true, "")
	if err != nil {
		return err
	}
	PQDBName, err := env.GetAsString("POSTGRES_DATABASE", true, "")
	if err != nil {
		return err
	}
	PQSSLMode, err := env.GetAsString("POSTGRES_SSL_MODE", false, "require")
	if err != nil {
		return err
	}
	return nil
}
