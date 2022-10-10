package _10

import (
	"database/sql"
	"errors"
	"go.uber.org/zap"
)

func V0x10x0(*sql.DB) error {
	zap.S().Infof("Applying migration 0.10.0")
	return errors.New("not implemented")
}
