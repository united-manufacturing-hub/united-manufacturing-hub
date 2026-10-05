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
	"encoding/binary"
	"encoding/hex"
	"hash"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	fsmv2datacontract "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/datacontract"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
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
		// Generate a simple hash from the structure
		hash := generateDataModelHash(dataModel)

		dataModelData[i] = models.DataModel{
			Name:          dataModel.Name,
			Description:   dataModel.Description,
			LatestVersion: latestVersionKey(dataModel.Versions),
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

// DataContractsV2FromConfig reports the data contracts section, the merged
// concept that folds a data model and the contract enforcing it into one entry.
//
// It reads only fullConfig.DataContractsV2. The dataModels and dataContracts
// sections are a separate, older concept and never appear here, so retiring
// them leaves this untouched.
func DataContractsV2FromConfig(ctx context.Context, configManager config.ConfigManager, logger *zap.SugaredLogger) ([]models.DataContractV2, error) {
	fullConfig, err := configManager.GetConfig(ctx, 0)
	if err != nil {
		logger.Warnf("Failed to get config for data contracts: %v", err)

		return []models.DataContractV2{}, err
	}

	contracts := make([]models.DataContractV2, 0, len(fullConfig.DataContractsV2))

	for _, dataContract := range fullConfig.DataContractsV2 {
		contracts = append(contracts, models.DataContractV2{
			Name:          dataContract.Name,
			Description:   dataContract.Description,
			LatestVersion: latestVersionKey(dataContract.Versions),
			Hash:          generateVersionsHash(dataContract.Name, dataContract.Versions),
			Versions:      contractVersions(dataContract.Name, dataContract.Versions),
		})
	}

	sort.Slice(contracts, func(i, j int) bool {
		return contracts[i].Name < contracts[j].Name
	})

	return contracts, nil
}

// AddDataContractHealth sets each version's health from the data contract
// monitor. It leaves health nil, so the Console falls back to the instance
// health, when the monitor is off or has not reported yet.
func AddDataContractHealth(ctx context.Context, contracts []models.DataContractV2, logger *zap.SugaredLogger) {
	client := fsmv2client.GetClient()
	if client == nil || len(contracts) == 0 {
		return
	}

	obs, freshness, err := fsmv2client.GetFresh[simple.Status[fsmv2datacontract.Status]](ctx, client, fsmv2datacontract.Ref, 3*fsmv2datacontract.PollInterval)
	if err != nil {
		logger.Warnw("data contract status: failed to read observed state", "error", err)

		return
	}

	if freshness != fsmv2client.Fresh && freshness != fsmv2client.Stale {
		return
	}

	for i := range contracts {
		for j := range contracts[i].Versions {
			version := &contracts[i].Versions[j]
			if version.Contract != "" {
				version.Health = versionHealth(obs.Status, freshness == fsmv2client.Stale, version.Contract+"-")
			}
		}
	}
}

// versionHealth judges the version whose subjects start with prefix. A degraded
// status that lists nothing is a failed poll, which leaves every version unknown.
func versionHealth(status simple.Status[fsmv2datacontract.Status], stale bool, prefix string) *models.Health {
	missing := status.Result.Missing
	i := slices.IndexFunc(missing, func(subject string) bool { return strings.HasPrefix(subject, prefix) })

	switch {
	case stale:
		return degradedHealth("The data contract monitor stopped reporting, so whether this version is enforced is unknown.")
	case slices.Contains(status.Result.Untranslated, prefix):
		return degradedHealth("This version could not be translated into a schema, so it is not enforced. The instance log says why.")
	case i >= 0:
		return degradedHealth(missing[i] + " is not registered in the Schema Registry, so it is not enforced.")
	case status.Degraded && len(missing) == 0 && len(status.Result.Untranslated) == 0:
		return degradedHealth(status.Reason)
	}

	return &models.Health{Message: "Every schema of this version is registered.", ObservedState: "active", DesiredState: "active", Category: models.Active}
}

func degradedHealth(reason string) *models.Health {
	return &models.Health{Message: reason, ObservedState: "degraded", DesiredState: "active", Category: models.Degraded}
}

// contractVersions lists a contract's versions in numeric order, each with the
// address it is published under. The address follows the platform convention
// _<name>_<version>, which is what the historian and the topic paths expect.
func contractVersions(name string, versions map[string]config.DataModelVersion) []models.DataContractV2Version {
	versionKeys := make([]string, 0, len(versions))
	for versionKey := range versions {
		versionKeys = append(versionKeys, versionKey)
	}

	sort.Slice(versionKeys, func(i, j int) bool {
		return parseVersionNumber(versionKeys[i]) < parseVersionNumber(versionKeys[j])
	})

	contractVersions := make([]models.DataContractV2Version, 0, len(versionKeys))
	for _, versionKey := range versionKeys {
		contractVersions = append(contractVersions, models.DataContractV2Version{
			Version:  versionKey,
			Contract: "_" + name + "_" + versionKey,
		})
	}

	return contractVersions
}

// parseVersionNumber parses a version string (e.g., "v1", "v2") to an integer.
func parseVersionNumber(versionStr string) int {
	if len(versionStr) < 2 {
		return 0
	}

	versionNum, err := strconv.Atoi(versionStr[1:])
	if err != nil {
		return 0
	}

	return versionNum
}

// latestVersionKey returns the highest "vN" key in a version map, falling back
// to an arbitrary key when none are versioned and to "" when there are none.
func latestVersionKey(versions map[string]config.DataModelVersion) string {
	latestVersion := ""
	highestVersion := 0

	for versionKey := range versions {
		if len(versionKey) > 1 && versionKey[0] == 'v' {
			if versionNum := parseVersionNumber(versionKey); versionNum > highestVersion {
				highestVersion = versionNum
				latestVersion = versionKey
			}
		}
	}

	// If no versioned keys found, use the first available key
	if latestVersion == "" {
		for versionKey := range versions {
			latestVersion = versionKey

			break
		}
	}

	return latestVersion
}

// generateDataModelHash generates a simple hash from the data model structure.
func generateDataModelHash(dataModel config.DataModelsConfig) string {
	return generateVersionsHash(dataModel.Name, dataModel.Versions)
}

// generateVersionsHash hashes a name together with every version key and the
// structure behind it, so two instances holding the same definition under the
// same name produce the same value and a drifted one does not.
//
// Every string is written with its length and every map with its entry count,
// so two different definitions can't produce the same byte stream: without
// them, a folder "a" holding field "b" and a field "ab" would both hash as
// "ab" plus the payload shape.
func generateVersionsHash(name string, versions map[string]config.DataModelVersion) string {
	if len(versions) == 0 {
		return ""
	}

	h := sha256.New()
	writeHashString(h, name)
	writeHashCount(h, len(versions))

	for _, versionKey := range sortedKeys(versions) {
		writeHashString(h, versionKey)
		hashStructure(h, versions[versionKey].Structure)
	}

	return hex.EncodeToString(h.Sum(nil))[:16] // Return first 16 characters
}

// hashStructure recursively hashes the structure map, fields in key order.
func hashStructure(h hash.Hash, structure map[string]config.Field) {
	writeHashCount(h, len(structure))

	for _, fieldKey := range sortedKeys(structure) {
		field := structure[fieldKey]
		writeHashString(h, fieldKey)
		writeHashString(h, field.PayloadShape)

		writeHashPresent(h, field.ModelRef != nil)

		if field.ModelRef != nil {
			writeHashString(h, field.ModelRef.Name)
			writeHashString(h, field.ModelRef.Version)
		}

		// The description is left out, as it is for the contract itself: it
		// does not change what messages are validated against.
		writeHashPresent(h, field.Relational != nil)

		if field.Relational != nil {
			hashPayloadFields(h, field.Relational.Fields)
		}

		hashStructure(h, field.Subfields)
	}
}

// hashPayloadFields recursively hashes the fields of an inline relational
// definition, in key order.
func hashPayloadFields(h hash.Hash, fields map[string]config.PayloadField) {
	writeHashCount(h, len(fields))

	for _, fieldKey := range sortedKeys(fields) {
		writeHashString(h, fieldKey)
		writeHashString(h, fields[fieldKey].Type)
		hashPayloadFields(h, fields[fieldKey].Subfields)
	}
}

func writeHashString(h hash.Hash, value string) {
	writeHashCount(h, len(value))
	h.Write([]byte(value))
}

func writeHashCount(h hash.Hash, count int) {
	h.Write(binary.BigEndian.AppendUint64(nil, uint64(count)))
}

func writeHashPresent(h hash.Hash, present bool) {
	if present {
		h.Write([]byte{1})
	} else {
		h.Write([]byte{0})
	}
}

func sortedKeys[V any](entries map[string]V) []string {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}
