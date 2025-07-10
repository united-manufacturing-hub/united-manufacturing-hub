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

package generator

import (
	"context"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// StreamProcessorsFromConfig extracts stream processors from the config and converts them to DFCs.
// TODO: This is a temporary solution. Once the stream processor FSM is ready, replace this with
// the proper snapshot-based method similar to ProtocolConvertersFromSnapshot.
func StreamProcessorsFromConfig(
	ctx context.Context,
	configManager config.ConfigManager,
	log *zap.SugaredLogger,
) []models.Dfc {
	config, err := configManager.GetConfig(ctx, 0)
	if err != nil {
		log.Warnf("Failed to get config for stream processors: %v", err)
		return []models.Dfc{}
	}

	var out []models.Dfc
	for _, sp := range config.StreamProcessor {
		dfc := buildStreamProcessorAsDfc(sp, log)
		out = append(out, dfc)
	}

	return out
}

// buildStreamProcessorAsDfc converts a StreamProcessorConfig to a models.Dfc with hardcoded health.
func buildStreamProcessorAsDfc(
	sp config.StreamProcessorConfig,
	log *zap.SugaredLogger,
) models.Dfc {
	// Generate UUID from stream processor name using deterministic method
	spUUID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(sp.Name))

	// Use hardcoded health as requested
	health := &models.Health{
		Message:       "Stream processor is configured",
		ObservedState: "configured",
		DesiredState:  "running",
		Category:      models.Neutral,
	}

	// Create DFC with stream processor type (using "custom" for now as there's no specific stream processor type)
	dfc := models.Dfc{
		Name:          &sp.Name,
		UUID:          spUUID.String(),
		Health:        health,
		Type:          models.DfcTypeStreamProcessor, // Using custom type for stream processors
		Metrics:       &models.DfcMetrics{},          // Empty metrics for now
		Connections:   []models.Connection{},         // No connections for stream processors
		IsInitialized: false,                         // Hardcoded as false since FSM is not ready
	}

	return dfc
}
