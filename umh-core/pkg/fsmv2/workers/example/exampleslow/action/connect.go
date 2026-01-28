// Copyright 2025 UMH Systems GmbH
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

package action

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow/snapshot"
)

const ConnectActionName = "connect"

// ConnectAction attempts to establish a connection with a configurable delay.
type ConnectAction struct{}

func (a *ConnectAction) Execute(ctx context.Context, depsAny any) error {
	deps := depsAny.(snapshot.ExampleslowDependencies)
	logger := deps.GetLogger()
	delaySeconds := deps.GetDelaySeconds()

	logger.Infow("connect_attempting", "delay_seconds", delaySeconds)

	if delaySeconds > 0 {
		select {
		case <-time.After(time.Duration(delaySeconds) * time.Second):
			logger.Info("Connect delay completed successfully")
		case <-ctx.Done():
			logger.Warn("Connect action cancelled during delay")

			return ctx.Err()
		}
	}

	logger.Info("Connect action completed")

	deps.SetConnected(true)

	return nil
}

func (a *ConnectAction) String() string {
	return ConnectActionName
}

func (a *ConnectAction) Name() string {
	return ConnectActionName
}
