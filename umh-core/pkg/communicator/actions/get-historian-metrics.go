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

// GetHistorianMetrics reads the state of the configured historian database on
// request: its versions, storage, compression and retention policies, background
// jobs, and per-table detail. It runs the queries when asked rather than on the
// connection monitor's tick, so nothing is published while nobody is looking and
// the reads carry no observation deadline.

package actions

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/historian/timescalemetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// HistorianMetricsCollector reads one database and returns what it found. The
// seam exists so the action can be tested without a database.
type HistorianMetricsCollector func(ctx context.Context, dsn string) (timescalemetrics.Metrics, error)

// GetHistorianMetricsAction implements the Action interface for reading the
// historian database state. All fields are immutable after construction.
type GetHistorianMetricsAction struct {
	configManager   config.ConfigManager
	collect         HistorianMetricsCollector
	outboundChannel chan *models.UMHMessage
	actionLogger    *zap.SugaredLogger

	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID
}

// NewGetHistorianMetricsAction returns an un-parsed action instance.
func NewGetHistorianMetricsAction(
	userEmail string,
	actionUUID uuid.UUID,
	instanceUUID uuid.UUID,
	outboundChannel chan *models.UMHMessage,
	configManager config.ConfigManager,
) *GetHistorianMetricsAction {
	return &GetHistorianMetricsAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		collect:         collectOverNewConnection,
		actionLogger:    logger.For(logger.ComponentCommunicator),
	}
}

// collectOverNewConnection dials the historian for this one request and hangs up
// again. One connection rather than a pool, because the reads are sequential on a
// single goroutine and nothing outlives the request. The connection monitor's own
// pool is not reused either: it belongs to a worker on its own goroutine and
// serves a liveness check every second, which a long catalog read must not block.
//
// pgx.Connect rather than pgxpool.New, because the pool connects lazily: it
// returns no error for an unreachable host, so a wrong host or password would be
// reported as whichever query happened to run first rather than as a failure to
// connect.
func collectOverNewConnection(ctx context.Context, dsn string) (timescalemetrics.Metrics, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return timescalemetrics.Metrics{}, fmt.Errorf("connect to the historian database: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	return timescalemetrics.Collect(ctx, conn)
}

// Parse implements the Action interface. GetHistorianMetrics carries no payload.
func (a *GetHistorianMetricsAction) Parse(_ interface{}) error {
	return nil
}

// Validate implements the Action interface. Nothing to validate for a read.
func (a *GetHistorianMetricsAction) Validate() error {
	return nil
}

// getUserEmail implements the Action interface.
func (a *GetHistorianMetricsAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface.
func (a *GetHistorianMetricsAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// Execute implements the Action interface by collecting and returning the
// historian database state.
func (a *GetHistorianMetricsAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing GetHistorianMetrics action")

	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	cfg, err := a.configManager.GetConfig(ctx, 0)
	if err != nil {
		return a.fail(fmt.Sprintf("Failed to read configuration: %v", err), models.ErrConfigFileInvalid)
	}

	if cfg.Historian == nil {
		return a.fail("No historian is configured on this instance", models.ErrHistorianMetricsFailed)
	}

	metrics, err := a.collect(ctx, cfg.Historian.Timescale.ToDSN())
	if err != nil {
		return a.fail(fmt.Sprintf("Failed to read the historian database: %v", err), models.ErrHistorianMetricsFailed)
	}

	// The terminal ActionFinishedSuccessfull reply is sent by the caller (see actions.go).
	return metrics, nil, nil
}

func (a *GetHistorianMetricsAction) fail(message string, code string) (interface{}, map[string]interface{}, error) {
	SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
		message, code, nil, a.outboundChannel, models.GetHistorianMetrics, nil)

	return nil, nil, errors.New(message)
}
