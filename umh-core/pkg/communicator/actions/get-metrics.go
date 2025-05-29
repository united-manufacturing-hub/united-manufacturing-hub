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

package actions

import (
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions/providers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

type GetMetricsAction struct {
	ActionDependencies
	payload  models.GetMetricsRequest
	provider providers.MetricsProvider
}

// NewGetMetricsAction creates a new GetMetricsAction with the provided parameters.
// Caller needs to invoke Parse and Validate before calling Execute.
func (a *ActionFactory) NewGetMetricsAction() *GetMetricsAction {
	return &GetMetricsAction{ActionDependencies: a.ActionDependencies, provider: &providers.DefaultMetricsProvider{}}
}

// For testing - allow injection of a custom provider
func (a *ActionFactory) NewGetMetricsActionWithProvider(provider providers.MetricsProvider) *GetMetricsAction {
	return &GetMetricsAction{ActionDependencies: a.ActionDependencies, provider: provider}
}

// Parse extracts the business fields from the raw JSON payload.
// Shape errors are detected here, while semantic validation is done in Validate.
func (a *GetMetricsAction) Parse(payload interface{}) (err error) {
	a.ActionLogger.Info("Parsing the payload")
	a.payload, err = ParseActionPayload[models.GetMetricsRequest](payload)
	a.ActionLogger.Infow("Payload parsed", "payload", a.payload)
	return err
}

// Validate performs semantic validation of the parsed payload.
// This verifies that the metric type is allowed and that the UUID is valid for DFC metrics.
func (a *GetMetricsAction) Validate() (err error) {
	a.ActionLogger.Info("Validating the payload")

	allowedMetricTypes := []models.MetricResourceType{models.DFCMetricResourceType, models.RedpandaMetricResourceType}
	if !slices.Contains(allowedMetricTypes, a.payload.Type) {
		return errors.New("metric type must be set and must be one of the following: dfc, redpanda")
	}

	if a.payload.Type == models.DFCMetricResourceType {
		if a.payload.UUID == "" {
			return errors.New("uuid must be set to retrieve metrics for a DFC")
		}

		_, err = uuid.Parse(a.payload.UUID)
		if err != nil {
			return fmt.Errorf("invalid UUID format: %v", err)
		}
	}

	return nil
}

// Execute retrieves the metrics from the correct source based on the metric type.
// It returns a response object with an array of metrics or an error if the retrieval fails.
func (a *GetMetricsAction) Execute() (interface{}, map[string]interface{}, error) {
	a.ActionLogger.Info("Executing the action")

	metrics, err := a.provider.GetMetrics(a.payload, a.SystemSnapshotManager.GetDeepCopySnapshot())
	if err != nil {
		a.SendFailure(err.Error(), models.GetMetrics)
		return nil, nil, err
	}

	return metrics, nil, nil
}

func (a *GetMetricsAction) getUserEmail() string {
	return a.UserEmail
}

func (a *GetMetricsAction) getUuid() uuid.UUID {
	return a.ActionUUID
}

func (a *GetMetricsAction) GetParsedPayload() models.GetMetricsRequest {
	return a.payload
}
