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

package permission_validator

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"sync"

	_ "embed"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// ValidateUserCertificateForAction validates that a user is authorized to perform an action based on their certificate.
func ValidateUserCertificateForAction(log *zap.SugaredLogger, cert *x509.Certificate, actionType *models.ActionType, messageType models.MessageType, instanceLocation map[int]string) error {
	log.Infof("Validating user certificate for action: %s, message type: %s", actionType, messageType)

	if cert == nil {
		log.Infof("No certificate found, skipping validation")

		return nil // No certificate means no authorization
	}

	// Extract the role from the certificate based on the current location
	role, err := GetRoleForLocation(cert, instanceLocation)
	if err != nil {
		log.Warnf("Failed to extract role from certificate: %v", err)
		// Continue without role check for now
		return err
	}

	if role == "" {
		var locationStr string
		for _, loc := range instanceLocation {
			locationStr = fmt.Sprintf("%s-%s", locationStr, loc)
		}

		return fmt.Errorf("user is not allowed to access this location: %s", locationStr)
	} else {
		log.Infof("User role from certificate: %s", role)
		// Additional role-based checks could be added here if needed
	}

	log.Infof("User is authorized to perform actions in this location")

	if !IsRoleAllowedForActionAndMessageType(role, actionType, messageType) {
		if actionType != nil {
			return fmt.Errorf("user is not authorized to perform actions of type %s and message type %s", *actionType, messageType)
		}

		return fmt.Errorf("user is not authorized to perform actions of message type %s", messageType)
	}

	return nil
}

// IsRoleAllowedForActionAndMessageType checks if a role is allowed to perform an action with a specific message type.
func IsRoleAllowedForActionAndMessageType(role Role, actionType *models.ActionType, messageType models.MessageType) bool {
	// If the action type is not set, the user is authorized
	if actionType == nil && messageType != models.Action {
		// Subscribe, Status, ActionReply, EncryptedContent are always allowed
		return true
	}

	// If we have an action type, check if the role is allowed for this action type
	if actionType != nil {
		if IsAllowedForAction(role, *actionType) {
			return true
		}
	}

	// Default deny if we get here
	return false
}

//go:embed action-type-role-map.json
var actionTypeRoleMapJSON []byte

// RoleMap represents the complete role mapping structure.
type RoleMap struct {
	Groups    map[string]ActionGroup `json:"groups"`
	Ungrouped UngroupedActions       `json:"ungrouped"`
}

// ActionGroup represents a group of related actions.
type ActionGroup struct {
	Actions     map[string][]string `json:"actions"`
	Description string              `json:"description"`
}

// UngroupedActions represents actions that don't belong to any group.
type UngroupedActions struct {
	Unknown []string `json:"unknown"`
	Dummy   []string `json:"dummy"`
}

// actionTypeRoleMapCache holds the cached role map.
var (
	roleMapCache     *RoleMap
	roleMapCacheMu   sync.RWMutex
	roleMapCacheInit sync.Once
)

// loadActionTypeRoleMap loads the action type to role mapping from the JSON file.
func loadActionTypeRoleMap() (*RoleMap, error) {
	// Check if we have a cached version
	roleMapCacheMu.RLock()

	if roleMapCache != nil {
		defer roleMapCacheMu.RUnlock()

		return roleMapCache, nil
	}

	roleMapCacheMu.RUnlock()

	// Initialize the cache once
	var initErr error

	roleMapCacheInit.Do(func() {
		// Parse the JSON into our new structure
		var roleMap RoleMap
		if err := json.Unmarshal(actionTypeRoleMapJSON, &roleMap); err != nil {
			initErr = fmt.Errorf("failed to parse role map: %w", err)

			return
		}

		// Store in cache
		roleMapCacheMu.Lock()

		roleMapCache = &roleMap

		roleMapCacheMu.Unlock()
	})

	if initErr != nil {
		return nil, initErr
	}

	roleMapCacheMu.RLock()
	defer roleMapCacheMu.RUnlock()

	return roleMapCache, nil
}

// IsAllowedForAction checks if a role is allowed to perform an action.
func IsAllowedForAction(role Role, actionType models.ActionType) bool {
	// Load the role map
	roleMap, err := loadActionTypeRoleMap()
	if err != nil {
		zap.S().Errorf("Failed to load role map: %v", err)

		return false
	}

	// Additional safety check to prevent nil pointer dereference
	if roleMap == nil {
		zap.S().Errorf("Role map is nil")

		return false
	}

	// Check in all groups
	for _, group := range roleMap.Groups {
		if allowedRoles, ok := group.Actions[string(actionType)]; ok {
			// Check if the user's role is in the allowed roles for this action type
			for _, allowedRole := range allowedRoles {
				if string(role) == allowedRole {
					return true
				}
			}
			// If we found the action but the role wasn't allowed, return false
			return false
		}
	}

	// Check in ungrouped actions
	if allowedRoles, ok := map[string][]string{
		"unknown": roleMap.Ungrouped.Unknown,
		"dummy":   roleMap.Ungrouped.Dummy,
	}[string(actionType)]; ok {
		for _, allowedRole := range allowedRoles {
			if string(role) == allowedRole {
				return true
			}
		}

		return false
	}

	// Default deny if we get here
	return false
}
