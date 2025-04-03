# UMH Action System

The actions package handles user-initiated operations within the UMH platform. Actions provide a standardized way to implement commands that can be triggered by users through the API.

## Action Interface

Every action must implement the Action interface which contains:

```go
type Action interface {
    Parse(payload interface{}) error
    Validate() error
    Execute() (interface{}, map[string]interface{}, error)
    getUserEmail() string
    getUuid() uuid.UUID
}
```

## Design Principles

1. **Single Responsibility**: Actions should only:
   - Read/write to configuration
   - Check system state
   - Return appropriate responses

2. **Separation of Concerns**: Complex business logic, especially long-running operations, should be implemented in finite state machines (FSMs) outside the communicator.

3. **Idempotence**: Actions should be idempotent when possible. The same action called multiple times should not cause unexpected side effects.

## Adding a New Action

### 1. Define the Action Type

First, add your new action type to the `ActionType` constants in `umh-core/pkg/models/action_models.go`:

```go
const (
    // ... existing action types
    MyNewAction ActionType = "my-new-action"
)
```

### 2. Create Payload Structures

Define any payload structures needed for your action in `umh-core/pkg/models/action_models.go`:

```go
type MyNewActionPayload struct {
    Name string `json:"name" binding:"required"`
    // Add other fields as needed
}
```

### 3. Implement the Action

Create a new file (e.g., `my_new_action.go`) in the actions package:

```go
package actions

import (
    "context"
    "errors"
    "fmt"

    "github.com/google/uuid"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
    "go.uber.org/zap"
)

// MyNewAction implements the Action interface for your specific functionality
type MyNewAction struct {
    userEmail       string
    actionUUID      uuid.UUID
    instanceUUID    uuid.UUID
    outboundChannel chan *models.UMHMessage
    configManager   config.ConfigManager
    
    // Action-specific fields
    name            string
}

// Constructor for dependency injection and testing
func NewMyNewAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, 
                   outboundChannel chan *models.UMHMessage, configManager config.ConfigManager) *MyNewAction {
    return &MyNewAction{
        userEmail:       userEmail,
        actionUUID:      actionUUID,
        instanceUUID:    instanceUUID,
        outboundChannel: outboundChannel,
        configManager:   configManager,
    }
}

// Parse implements the Action interface
func (a *MyNewAction) Parse(payload interface{}) error {
    zap.S().Debug("Parsing MyNewAction payload")

    // Convert the payload to a map
    payloadMap, ok := payload.(map[string]interface{})
    if !ok {
        return errors.New("invalid payload format, expected map")
    }

    // Extract and validate fields
    name, ok := payloadMap["name"].(string)
    if !ok || name == "" {
        return errors.New("missing or invalid name")
    }
    
    a.name = name
    return nil
}

// Validate implements the Action interface
func (a *MyNewAction) Validate() error {
    // Validate the action's fields
    if a.name == "" {
        return errors.New("name cannot be empty")
    }
    
    return nil
}

// Execute implements the Action interface
func (a *MyNewAction) Execute() (interface{}, map[string]interface{}, error) {
    zap.S().Info("Executing MyNewAction")

    // Send confirmation that action is starting
    SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed, 
                   "Starting MyNewAction", a.outboundChannel, models.MyNewAction)

    // Send progress update
    SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, 
                   "Processing request", a.outboundChannel, models.MyNewAction)

    // Perform the action's operation (typically config changes)
    err := a.configManager.SomeConfigOperation(context.Background(), a.name)
    if err != nil {
        errorMsg := fmt.Sprintf("Failed to execute action: %s", err)
        SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, 
                       errorMsg, a.outboundChannel, models.MyNewAction)
        return nil, nil, fmt.Errorf("failed to execute action: %w", err)
    }

    // Send success message
    SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedSuccessfull, 
                   "Operation completed successfully", a.outboundChannel, models.MyNewAction)

    return "Successfully executed action", nil, nil
}

// getUserEmail implements the Action interface
func (a *MyNewAction) getUserEmail() string {
    return a.userEmail
}

// getUuid implements the Action interface
func (a *MyNewAction) getUuid() uuid.UUID {
    return a.actionUUID
}
```

## Waiting for Observed State Changes

For actions that need to wait for system state changes, implement a polling or observer pattern in the Execute method:

```go
func (a *MyActionWithWaiting) Execute() (interface{}, map[string]interface{}, error) {
    // Initial setup and confirmation
    SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed, 
                   "Starting action", a.outboundChannel, models.MyActionWithWaiting)
    
    // Apply config changes
    err := a.configManager.ApplyChange(context.Background(), a.parameters)
    if err != nil {
        // Handle error
        return nil, nil, err
    }
    
    // Wait for state change with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()
    
    // Poll for state change
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return nil, nil, fmt.Errorf("timeout waiting for state change")
        case <-ticker.C:
            // Check system state
            currentState, err := a.stateChecker.GetCurrentState()
            if err != nil {
                continue
            }
            
            // If desired state is reached
            if currentState == a.desiredState {
                SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, 
                               models.ActionFinishedSuccessfull, "State change completed", 
                               a.outboundChannel, models.MyActionWithWaiting)
                return "Successfully changed state", nil, nil
            }
            
            // Send progress update
            SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, 
                           fmt.Sprintf("Waiting for state change, current: %s", currentState), 
                           a.outboundChannel, models.MyActionWithWaiting)
        }
    }
}
```

## Unit Testing Actions

Actions should be designed for testability with dependency injection. Here's an example of how to test an action:

```go
func TestMyNewAction_Execute(t *testing.T) {
    // Setup
    mockConfigManager := mocks.NewMockConfigManager(t)
    outboundChannel := make(chan *models.UMHMessage, 10)
    
    action := NewMyNewAction(
        "test@example.com",
        uuid.New(),
        uuid.New(),
        outboundChannel,
        mockConfigManager,
    )
    
    // Set action fields
    action.name = "test-name"
    
    // Setup expectations
    mockConfigManager.EXPECT().
        SomeConfigOperation(gomock.Any(), "test-name").
        Return(nil)
    
    // Execute
    result, context, err := action.Execute()
    
    // Assert
    assert.NoError(t, err)
    assert.NotNil(t, result)
    
    // Verify messages sent
    assert.Equal(t, 3, len(outboundChannel)) // Confirm + Executing + Success
    
    // Check message content (optional)
    msg := <-outboundChannel // Confirm message
    assert.Equal(t, models.ActionConfirmed, getActionReplyState(msg))
    
    msg = <-outboundChannel // Executing message
    assert.Equal(t, models.ActionExecuting, getActionReplyState(msg))
    
    msg = <-outboundChannel // Success message
    assert.Equal(t, models.ActionFinishedSuccessfull, getActionReplyState(msg))
}

// Helper function to extract action reply state from UMH message
func getActionReplyState(msg *models.UMHMessage) models.ActionReplyState {
    // Parse message to get action reply state
    // ...
}
```

## Best Practices

1. **Keep Actions Focused**: Actions should focus on configuration changes and system state checks, delegating complex business logic to FSMs.

2. **Provide Feedback**: Use `SendActionReply` to keep users informed about action progress.

3. **Clean Error Handling**: Provide clear error messages both to logs and user feedback.

4. **Make Actions Testable**: Use dependency injection for external dependencies like config managers.

5. **Rate Limiting**: Consider implementing rate limiting for actions that could overload the system.

6. **Audit Logging**: Log all action executions for audit purposes.

7. **Transaction Safety**: When possible, make config changes atomically or with rollback capability. 