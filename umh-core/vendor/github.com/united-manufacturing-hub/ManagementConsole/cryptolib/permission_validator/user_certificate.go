package permission_validator

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/united-manufacturing-hub/ManagementConsole/cryptolib/certificate"
	"go.uber.org/zap"
)

// IsAllowedForAction checks if a role is allowed to perform an action.
func IsAllowedForAction(role certificate.Role, actionType string) bool {
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
		if allowedRoles, ok := group.Actions[actionType]; ok {
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
	}[actionType]; ok {
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

//go:embed action-type-role-map.json
var actionTypeRoleMapJSON []byte

// RoleMap represents the complete role mapping structure
type RoleMap struct {
	Groups    map[string]ActionGroup `json:"groups"`
	Ungrouped UngroupedActions       `json:"ungrouped"`
}

// ActionGroup represents a group of related actions
type ActionGroup struct {
	Description string              `json:"description"`
	Actions     map[string][]string `json:"actions"`
}

// UngroupedActions represents actions that don't belong to any group
type UngroupedActions struct {
	Unknown []string `json:"unknown"`
	Dummy   []string `json:"dummy"`
}

// actionTypeRoleMapCache holds the cached role map
var (
	roleMapCache     *RoleMap
	roleMapCacheMu   sync.RWMutex
	roleMapCacheInit sync.Once
)

// loadActionTypeRoleMap loads the action type to role mapping from the JSON file
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
