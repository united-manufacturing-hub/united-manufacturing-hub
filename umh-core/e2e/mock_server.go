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

package e2e_test

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/backend_api_structs"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

const DefaultPushBufferSize = 100

// MockAPIServer provides a simple mock implementation of the UMH backend API
// that umh-core expects for communication
type MockAPIServer struct {
	server *http.Server
	router *gin.Engine
	port   int
	logger *zap.SugaredLogger

	// Queue for messages to be delivered to umh-core when it pulls
	pullQueue   []models.UMHMessage
	pullQueueMu sync.RWMutex

	// Channel for collecting messages that umh-core pushes to us
	PushedMessages chan models.UMHMessage

	// Simple auth tracking
	validTokens map[string]string // token -> instance name
	tokensMu    sync.RWMutex

	// Instance tracking
	instanceUUID uuid.UUID
	instanceName string
}

// NewMockAPIServer creates a new mock API server on a random available port
func NewMockAPIServer() *MockAPIServer {
	gin.SetMode(gin.TestMode)

	port := getAvailablePort()

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(fmt.Sprintf("failed to create logger: %v", err))
	}

	server := &MockAPIServer{
		router:         gin.New(),
		port:           port,
		logger:         logger.Sugar(),
		pullQueue:      make([]models.UMHMessage, 0),
		PushedMessages: make(chan models.UMHMessage, DefaultPushBufferSize),
		validTokens:    make(map[string]string),
		instanceUUID:   uuid.New(),
		instanceName:   "test-instance",
	}

	server.setupRoutes()
	server.setupHTTPServer()

	return server
}

func (s *MockAPIServer) setupRoutes() {
	// Add basic middleware
	s.router.Use(gin.Recovery())
	s.router.Use(s.loggingMiddleware())

	// UMH Core API endpoints
	s.router.POST("/v2/instance/login", s.handleLogin)
	s.router.GET("/v2/instance/pull", s.handlePull)
	s.router.POST("/v2/instance/push", s.handlePush)
}

func (s *MockAPIServer) setupHTTPServer() {
	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

func (s *MockAPIServer) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)

		if raw != "" {
			path = path + "?" + raw
		}

		s.logger.Infow("API Request",
			"method", c.Request.Method,
			"path", path,
			"status", c.Writer.Status(),
			"latency", latency,
		)
	}
}

// Start starts the mock API server
func (s *MockAPIServer) Start() error {
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Errorw("Mock API server failed", "error", err)
		}
	}()

	// Verify server is responding
	for i := 0; i < 50; i++ {
		if resp, err := http.Get(s.GetURL() + "/v2/instance/login"); err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	s.logger.Infow("Mock API server started", "port", s.port, "url", s.GetURL())
	return nil
}

// Stop stops the mock API server
func (s *MockAPIServer) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// GetURL returns the base URL of the mock server
func (s *MockAPIServer) GetURL() string {
	return fmt.Sprintf("http://localhost:%d", s.port)
}

// GetHostURL returns the base URL that can be accessed from inside a Docker container
func (s *MockAPIServer) GetHostURL() string {
	return fmt.Sprintf("http://host.docker.internal:%d", s.port)
}

// GetPort returns the port the server is listening on
func (s *MockAPIServer) GetPort() int {
	return s.port
}

// AddMessageToPullQueue adds a message to the queue that will be delivered to umh-core when it pulls
func (s *MockAPIServer) AddMessageToPullQueue(message models.UMHMessage) {
	s.pullQueueMu.Lock()
	defer s.pullQueueMu.Unlock()

	s.pullQueue = append(s.pullQueue, message)
	s.logger.Infow("Added message to pull queue", "content", message.Content, "queue_size", len(s.pullQueue))
}

// GetPushedMessageCount returns the number of messages umh-core has pushed to us
func (s *MockAPIServer) GetPushedMessageCount() int {
	return len(s.PushedMessages)
}

// DrainPushedMessages returns all pushed messages and clears the channel
func (s *MockAPIServer) DrainPushedMessages() []models.UMHMessage {
	var messages []models.UMHMessage
	for {
		select {
		case msg := <-s.PushedMessages:
			messages = append(messages, msg)
		default:
			return messages
		}
	}
}

// handleLogin handles the /v2/instance/login endpoint
func (s *MockAPIServer) handleLogin(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
		return
	}

	// For testing, we accept any Bearer token
	if len(authHeader) < 7 || authHeader[:7] != "Bearer " {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
		return
	}

	token := authHeader[7:]

	// Generate a simple JWT token for testing
	jwtToken := fmt.Sprintf("mock-jwt-%d", time.Now().Unix())

	// Store the token for validation in pull/push requests
	s.tokensMu.Lock()
	s.validTokens[jwtToken] = s.instanceName
	s.tokensMu.Unlock()

	// Set the JWT as a cookie (as expected by umh-core)
	c.SetCookie("token", jwtToken, 3600, "/", "", false, true)

	// Return the login response
	response := backend_api_structs.InstanceLoginResponse{
		UUID: s.instanceUUID.String(),
		Name: s.instanceName,
	}

	s.logger.Infow("Login successful", "token", token, "jwt", jwtToken, "instance", s.instanceName)
	c.JSON(http.StatusOK, response)
}

// handlePull handles the /v2/instance/pull endpoint
func (s *MockAPIServer) handlePull(c *gin.Context) {
	// Validate JWT token from cookie
	jwtToken, err := c.Cookie("token")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token cookie"})
		return
	}

	s.tokensMu.RLock()
	_, valid := s.validTokens[jwtToken]
	s.tokensMu.RUnlock()

	if !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	// Get messages from the pull queue
	s.pullQueueMu.Lock()
	messages := make([]models.UMHMessage, len(s.pullQueue))
	copy(messages, s.pullQueue)
	s.pullQueue = s.pullQueue[:0] // Clear the queue
	s.pullQueueMu.Unlock()

	response := backend_api_structs.PullPayload{
		UMHMessages: messages,
	}

	if len(messages) > 0 {
		s.logger.Infow("Delivered messages to umh-core", "count", len(messages))
	}

	c.JSON(http.StatusOK, response)
}

// handlePush handles the /v2/instance/push endpoint
func (s *MockAPIServer) handlePush(c *gin.Context) {
	// Validate JWT token from cookie
	jwtToken, err := c.Cookie("token")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token cookie"})
		return
	}

	s.tokensMu.RLock()
	_, valid := s.validTokens[jwtToken]
	s.tokensMu.RUnlock()

	if !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	// Parse the push payload
	var payload backend_api_structs.PushPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Store the pushed messages in our channel for testing verification
	for _, message := range payload.UMHMessages {
		// Try to decode and log status messages with detailed information
		s.logMessageDetails(message)

		select {
		case s.PushedMessages <- message:
			// Basic log kept for compatibility
			s.logger.Infow("Received pushed message", "content_length", len(message.Content), "from_instance", message.InstanceUUID, "email", message.Email)
		default:
			s.logger.Warnw("Push message channel full, dropping message", "content_length", len(message.Content), "email", message.Email)
		}
	}

	s.logger.Infow("Received push from umh-core", "message_count", len(payload.UMHMessages))
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// logMessageDetails decodes and logs detailed information about messages, especially status messages
func (s *MockAPIServer) logMessageDetails(message models.UMHMessage) {
	// Decode the base64 encoded message content
	content, err := encoding.DecodeMessageFromUMHInstanceToUser(message.Content)
	if err != nil {
		s.logger.Debugw("Could not decode message content", "error", err, "email", message.Email)
		return
	}

	s.logger.Infow("Decoded message",
		"type", content.MessageType,
		"email", message.Email,
		"instance", message.InstanceUUID)

	// If it's a status message, log detailed component health information
	if content.MessageType == models.Status {
		s.logStatusMessage(content, message.Email)
	}
}

// logStatusMessage logs detailed information about status messages
func (s *MockAPIServer) logStatusMessage(content models.UMHMessageContent, email string) {
	// Try to parse the status message payload
	statusMsg := parseStatusMessageFromPayload(content.Payload)
	if statusMsg == nil {
		s.logger.Warnw("Could not parse status message payload", "email", email)
		return
	}

	s.logger.Infow("=== STATUS MESSAGE RECEIVED ===",
		"email", email,
		"timestamp", time.Now().Format(time.RFC3339))

	// Log Core health
	if statusMsg.Core.Health != nil {
		s.logger.Infow("Core Status",
			"state", statusMsg.Core.Health.ObservedState,
			"desired_state", statusMsg.Core.Health.DesiredState,
			"category", statusMsg.Core.Health.Category,
			"message", statusMsg.Core.Health.Message)
	}

	// Log Agent health
	if statusMsg.Core.Agent.Health != nil {
		s.logger.Infow("Agent Status",
			"state", statusMsg.Core.Agent.Health.ObservedState,
			"desired_state", statusMsg.Core.Agent.Health.DesiredState,
			"category", statusMsg.Core.Agent.Health.Category,
			"message", statusMsg.Core.Agent.Health.Message)
	}

	// Log Redpanda health
	if statusMsg.Core.Redpanda.Health != nil {
		s.logger.Infow("Redpanda Status",
			"state", statusMsg.Core.Redpanda.Health.ObservedState,
			"desired_state", statusMsg.Core.Redpanda.Health.DesiredState,
			"category", statusMsg.Core.Redpanda.Health.Category,
			"message", statusMsg.Core.Redpanda.Health.Message,
			"incoming_throughput", statusMsg.Core.Redpanda.AvgIncomingThroughputPerMinuteInBytesSec,
			"outgoing_throughput", statusMsg.Core.Redpanda.AvgOutgoingThroughputPerMinuteInBytesSec)
	}

	// Log TopicBrowser health
	if statusMsg.Core.TopicBrowser.Health != nil {
		s.logger.Infow("TopicBrowser Status",
			"state", statusMsg.Core.TopicBrowser.Health.ObservedState,
			"desired_state", statusMsg.Core.TopicBrowser.Health.DesiredState,
			"category", statusMsg.Core.TopicBrowser.Health.Category,
			"message", statusMsg.Core.TopicBrowser.Health.Message,
			"topic_count", statusMsg.Core.TopicBrowser.TopicCount)
	}

	// Log Container health if available
	if statusMsg.Core.Container.Health != nil {
		s.logger.Infow("Container Status",
			"state", statusMsg.Core.Container.Health.ObservedState,
			"desired_state", statusMsg.Core.Container.Health.DesiredState,
			"category", statusMsg.Core.Container.Health.Category,
			"message", statusMsg.Core.Container.Health.Message,
			"architecture", statusMsg.Core.Container.Architecture)
	}

	// Log DFC count
	s.logger.Infow("DFC Information", "dfc_count", len(statusMsg.Core.Dfcs))

	s.logger.Infow("=== END STATUS MESSAGE ===")
}
