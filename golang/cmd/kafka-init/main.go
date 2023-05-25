// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"net"
	"time"
)

func main() {
	// Initialize zap logging
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION") //nolint:errcheck
	log := logger.New(logLevel)
	defer func(log *zap.SugaredLogger) {
		_ = log.Sync() //nolint:errcheck
	}(log)

	internal.Initfgtrace()

	kafkaBroker, err := env.GetAsString("KAFKA_BOOTSTRAP_SERVER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	zap.S().Infof("kaflaBroker: %s", kafkaBroker)

	timeout := 10 * time.Second
	conn, err := net.DialTimeout("tcp", kafkaBroker, timeout)
	if err != nil {
		zap.S().Errorf("Site unreachable. Error: %v", err)
	} else {
		zap.S().Info("Site reachable")
	}
	defer func(conn net.Conn) {
		err = conn.Close()
		if err != nil {
			zap.S().Errorf("Error closing connection: %s", err)
		}
	}(conn)

	Init(kafkaBroker)
}
