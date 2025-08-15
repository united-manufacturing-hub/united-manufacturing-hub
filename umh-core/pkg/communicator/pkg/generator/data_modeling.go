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
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"sort"
	"strconv"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// DataModelsFromConfig extracts data models from the configuration and converts them to status message format.
func DataModelsFromConfig(ctx context.Context, configManager config.ConfigManager, logger *zap.SugaredLogger) ([]models.DataModel, error) {
	// Get the full config and extract data models from it
	fullConfig, err := configManager.GetConfig(ctx, 0)
	if err != nil {
		logger.Warnf("Failed to get config for data models: %v", err)

		return []models.DataModel{}, err
	}

	dataModels := fullConfig.DataModels
	dataModelData := make([]models.DataModel, len(dataModels))

	for i, dataModel := range dataModels {
		// Extract the latest version from the versions map
		latestVersion := ""

		if len(dataModel.Versions) > 0 {
			// Find the highest version number
			highestVersion := 0

			for versionKey := range dataModel.Versions {
				if len(versionKey) > 1 && versionKey[0] == 'v' {
					if versionNum := parseVersionNumber(versionKey); versionNum > highestVersion {
						highestVersion = versionNum
						latestVersion = versionKey
					}
				}
			}
			// If no versioned keys found, use the first available key
			if latestVersion == "" {
				for versionKey := range dataModel.Versions {
					latestVersion = versionKey

					break
				}
			}
		}

		// Generate a simple hash from the structure
		hash := generateDataModelHash(dataModel)

		dataModelData[i] = models.DataModel{
			Name:          dataModel.Name,
			Description:   dataModel.Description,
			LatestVersion: latestVersion,
			Hash:          hash,
		}
	}

	return dataModelData, nil
}

// DataContractsFromConfig extracts data contracts from the configuration and converts them to status message format.
func DataContractsFromConfig(ctx context.Context, configManager config.ConfigManager, logger *zap.SugaredLogger) ([]models.DataContract, error) {
	fullConfig, err := configManager.GetConfig(ctx, 0)
	if err != nil {
		logger.Warnf("Failed to get config for data contracts: %v", err)

		return []models.DataContract{}, err
	}

	dataContracts := fullConfig.DataContracts
	dataContractData := make([]models.DataContract, len(dataContracts))

	for i, dataContract := range dataContracts {
		var dataModelRef models.DataContractRef
		if dataContract.Model != nil {
			dataModelRef = models.DataContractRef{
				Name:    dataContract.Model.Name,
				Version: dataContract.Model.Version,
			}
		}

		dataContractData[i] = models.DataContract{
			Name:      dataContract.Name,
			DataModel: dataModelRef,
			Flows:     0, // Set to 0 for now (TODO: add flows)
		}
	}

	return dataContractData, nil
}

// parseVersionNumber parses a version string (e.g., "v1", "v2") to an integer.
func parseVersionNumber(versionStr string) int {
	versionNum, err := strconv.Atoi(versionStr[1:])
	if err != nil {
		return 0
	}

	return versionNum
}

// generateDataModelHash generates a simple hash from the data model structure.
func generateDataModelHash(dataModel config.DataModelsConfig) string {
	if len(dataModel.Versions) == 0 {
		return ""
	}

	// Create a hash from the data model name and version content
	h := sha256.New()
	h.Write([]byte(dataModel.Name))

	// Sort version keys for deterministic hashing
	versionKeys := make([]string, 0, len(dataModel.Versions))
	for versionKey := range dataModel.Versions {
		versionKeys = append(versionKeys, versionKey)
	}

	sort.Strings(versionKeys)

	// Add each version and its content to the hash
	for _, versionKey := range versionKeys {
		version := dataModel.Versions[versionKey]
		h.Write([]byte(versionKey))

		// Hash the structure content
		hashStructure(h, version.Structure)
	}

	return hex.EncodeToString(h.Sum(nil))[:16] // Return first 16 characters
}

// hashStructure recursively hashes the structure map.
func hashStructure(h hash.Hash, structure map[string]config.Field) {
	if len(structure) == 0 {
		return
	}

	// Sort field keys for deterministic hashing
	fieldKeys := make([]string, 0, len(structure))
	for fieldKey := range structure {
		fieldKeys = append(fieldKeys, fieldKey)
	}

	sort.Strings(fieldKeys)

	// Hash each field and its content
	for _, fieldKey := range fieldKeys {
		field := structure[fieldKey]
		h.Write([]byte(fieldKey))
		h.Write([]byte(field.PayloadShape))

		// Handle ModelRef which is now a struct pointer
		if field.ModelRef != nil {
			h.Write([]byte(field.ModelRef.Name))
			h.Write([]byte(field.ModelRef.Version))
		}

		// Recursively hash subfields
		hashStructure(h, field.Subfields)
	}
}
