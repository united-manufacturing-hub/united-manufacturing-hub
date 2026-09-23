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

// GetHistorianMetrics reads the configured historian database on request:
// versions, storage, policies, background jobs and per-table detail. Nothing runs
// until a caller asks.

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
	deps "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/historian/timescalemetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// GetHistorianMetricsAction implements the Action interface for reading the
// historian database state. All fields are immutable after construction.
type GetHistorianMetricsAction struct {
	configManager   config.ConfigManager
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
		actionLogger:    logger.For(logger.ComponentCommunicator),
	}
}

// pgx.Connect dials here, so an unreachable host or a rejected password is
// reported as a failure to connect; a pool would defer the dial to its first
// acquire and surface it as a failed query.
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
		return a.fail(fmt.Sprintf("Failed to read configuration: %v", err),
			models.ErrConfigFileInvalid, err, "historian_metrics_config_read_failed")
	}

	// A client mistake, not an instance fault: reporting it would fill Sentry with
	// misrouted requests.
	if cfg.Historian == nil {
		return a.fail("No historian is configured on this instance",
			models.ErrHistorianMetricsFailed, nil, "")
	}

	metrics, err := collectOverNewConnection(ctx, cfg.Historian.Timescale.ToDSN())
	if err != nil {
		return a.fail(fmt.Sprintf("Failed to read the historian database: %v", err),
			models.ErrHistorianMetricsFailed, err, "historian_metrics_read_failed")
	}

	// The terminal ActionFinishedSuccessfull reply is sent by the caller (see actions.go).
	return metrics, nil, nil
}

// fail replies to the caller and, when cause is non-nil, reports to Sentry, so a
// database nobody can read does not wait for someone to notice an empty page.
func (a *GetHistorianMetricsAction) fail(
	message string,
	code string,
	cause error,
	event string,
) (interface{}, map[string]interface{}, error) {
	if cause != nil {
		communicatorFSMLogger().SentryError(
			deps.FeatureSupportHistorian, communicatorHierarchyPath, cause, event)
	}

	SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
		message, code, nil, a.outboundChannel, models.GetHistorianMetrics, nil)

	return nil, nil, errors.New(message)
}
