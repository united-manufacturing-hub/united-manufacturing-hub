# UMH Action System

The actions package handles user-initiated operations within the UMH platform. Actions provide a standardized way to implement commands that can be triggered by users via the Management Console.

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

Actions should be designed for testability with dependency injection. UMH uses custom mock implementations rather than mocking frameworks. Here's an example of how to test an action using Ginkgo and Gomega:

```go
var _ = Describe("MyNewAction", func() {
    // Variables used across tests
    var (
        action          *actions.MyNewAction
        userEmail       string
        actionUUID      uuid.UUID
        instanceUUID    uuid.UUID
        outboundChannel chan *models.UMHMessage
        mockConfig      *config.MockConfigManager
    )

    // Setup before each test
    BeforeEach(func() {
        // Initialize test variables
        userEmail = "test@example.com"
        actionUUID = uuid.New()
        instanceUUID = uuid.New()
        outboundChannel = make(chan *models.UMHMessage, 10) // Buffer to prevent blocking

        // Create initial config
        initialConfig := config.FullConfig{
            Agent: config.AgentConfig{
                MetricsPort: 8080,
                CommunicatorConfig: config.CommunicatorConfig{
                    APIURL:    "https://example.com",
                    AuthToken: "test-token",
                },
            },
        }

        // Set up mock with initial config
        mockConfig = config.NewMockConfigManager().WithConfig(initialConfig)
        action = actions.NewMyNewAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig)
    })

    // Cleanup after each test
    AfterEach(func() {
        // Drain the outbound channel to prevent goroutine leaks
        for len(outboundChannel) > 0 {
            <-outboundChannel
        }
        close(outboundChannel)
    })

    Describe("Execute", func() {
        It("should execute successfully", func() {
            // Set up the action with required data
            action.SetName("test-name")

            // Reset tracking for this test
            mockConfig.ResetCalls()

            // Execute the action
            result, metadata, err := action.Execute()
            Expect(err).NotTo(HaveOccurred())
            Expect(result).To(ContainSubstring("Successfully executed"))
            Expect(metadata).To(BeNil())

            // Verify that GetConfig was called
            Expect(mockConfig.GetConfigCalled).To(BeTrue())

            // Check that config was updated correctly
            updatedConfig, _ := mockConfig.GetConfig(nil, 0)
            Expect(updatedConfig.SomeConfigField).To(Equal("test-name"))

            // Verify messages sent (3 messages: Confirmed + Executing + Success)
            Expect(outboundChannel).To(HaveLen(3))

            // We can also extract and check message content
            var messages []*models.UMHMessage
            for i := 0; i < 3; i++ {
                select {
                case msg := <-outboundChannel:
                    messages = append(messages, msg)
                case <-time.After(100 * time.Millisecond):
                    Fail("Timed out waiting for message")
                }
            }
            
            // Verify message types (could extract and check ActionReplyState)
            // First message should be Confirmation
            // Second message should be Executing
            // Third message should be Success
        })

        It("should handle config errors", func() {
            // Set up mock to fail on GetConfig
            mockConfig.WithConfigError(errors.New("mock GetConfig failure"))
            
            // Set up the action with required data
            action.SetName("test-name")

            // Execute the action - should fail
            result, metadata, err := action.Execute()
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("failed to execute action"))
            Expect(result).To(BeNil())
            Expect(metadata).To(BeNil())

            // We should have 3 messages in the channel (Confirmed + Executing + Failure)
            Expect(outboundChannel).To(HaveLen(3))
        })
        
        It("should handle custom error cases", func() {
            // For testing more complex failure cases, we can create custom mock wrappers
            // similar to the writeFailingMockConfigManager in edit_instance_test.go
            
            customMock := &customFailingMockManager{
                mockConfigManager: mockConfig,
            }
            
            // Create new action with our custom mock
            action = actions.NewMyNewAction(userEmail, actionUUID, instanceUUID, outboundChannel, customMock)
            action.SetName("test-name")
            
            // Execute should fail due to our custom mock
            result, metadata, err := action.Execute()
            Expect(err).To(HaveOccurred())
            Expect(result).To(BeNil())
        })
    })
})

// Example of a custom mock wrapper for testing failure cases
type customFailingMockManager struct {
    mockConfigManager *config.MockConfigManager
}

func (c *customFailingMockManager) GetConfig(ctx context.Context, tick uint64) (config.FullConfig, error) {
    return c.mockConfigManager.GetConfig(ctx, tick)
}

func (c *customFailingMockManager) WriteConfig(ctx context.Context, config config.FullConfig) error {
    return errors.New("mock failure")
}

// Implement other required methods...
```

This testing approach focuses on:

1. **State Verification**: Checking the actual changes made to the configuration.
2. **Message Verification**: Ensuring the right messages are sent in the right order.
3. **Error Handling**: Testing both success and failure paths with custom mock wrappers.
4. **Ginkgo/Gomega Testing**: Using the BDD-style testing framework preferred by the UMH project.

## Best Practices

1. **Keep Actions Focused**: Actions should focus on configuration changes and system state checks, delegating complex business logic to FSMs.

2. **Provide Feedback**: Use `SendActionReply` to keep users informed about action progress.

3. **Clean Error Handling**: Provide clear error messages both to logs and user feedback.

4. **Make Actions Testable**: Use dependency injection for external dependencies like config managers.

5. **Transaction Safety**: When possible, make config changes atomically or with rollback capability. 
