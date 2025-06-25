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

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/pprof"
	v2 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/communication_state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/graphql"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/control"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/version"
	"go.uber.org/zap"
)

func main() {
	// Initialize the global logger first thing
	logger.Initialize()

	// Initialize Sentry
	sentry.InitSentry(version.GetAppVersion(), true)

	// Get a logger for the main component
	log := logger.For(logger.ComponentCore)

	// Log using the component logger with structured fields
	log.Info("Starting umh-core...")

	// Start the pprof server (if enabled)
	pprof.StartPprofServer()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load the config
	configManager, err := config.NewFileConfigManagerWithBackoff()
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeFatal, log, "Failed to create config manager: %w", err)
		os.Exit(1)
	}

	// Load or create configuration with environment variable overrides
	// This loads the config file if it exists, applies any environment variables as overrides,
	// and persists the result back to the config file. See detailed docs in config.LoadConfigWithEnvOverrides.
	configData, err := config.LoadConfigWithEnvOverrides(ctx, configManager, log)

	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeFatal, log, "Failed to load config: %w", err)
		os.Exit(1)
	}

	// Start the metrics server
	server := metrics.SetupMetricsEndpoint(fmt.Sprintf(":%d", configData.Agent.MetricsPort))
	defer func() {
		// S6_KILL_FINISH_MAXTIME is 5 seconds, so we need to finish before that
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			sentry.ReportIssuef(sentry.IssueTypeError, log, "Failed to shutdown metrics server: %w", err)
		}
	}()

	// Start the control loop
	controlLoop := control.NewControlLoop(configManager)
	systemSnapshotManager := controlLoop.GetSnapshotManager()

	// Initialize the communication state
	communicationState := communication_state.NewCommunicationState(
		watchdog.NewWatchdog(ctx, time.NewTicker(time.Second*10), true, logger.For(logger.ComponentCommunicator)),
		make(chan *models.UMHMessage, 100),
		make(chan *models.UMHMessage, 100),
		configData.Agent.ReleaseChannel,
		systemSnapshotManager,
		configManager,
		configData.Agent.APIURL,
		logger.For(logger.ComponentCommunicator),
		configData.Agent.AllowInsecureTLS,
		topicbrowser.NewCache(),
	)

	// Start the topic browser cache updater independent of the backend connection (e.g., for HTTP endpoints)
	// it updates the TopicBrowserCache based on the observed state of the topic browser service once per second
	communicationState.StartTopicBrowserCacheUpdater(systemSnapshotManager, ctx)

	// Start the GraphQL server if enabled
	var graphqlServer *http.Server
	if configData.Agent.GraphQLConfig.Enabled {
		// Set defaults for GraphQL config if not specified
		if configData.Agent.GraphQLConfig.Port == 0 {
			configData.Agent.GraphQLConfig.Port = 8090
		}
		if len(configData.Agent.GraphQLConfig.CORSOrigins) == 0 {
			configData.Agent.GraphQLConfig.CORSOrigins = []string{"*"}
		}

		// GraphQL server uses real data from the simulator via TopicBrowserCache

		graphqlResolver := &graphql.Resolver{
			SnapshotManager:   systemSnapshotManager,
			TopicBrowserCache: communicationState.TopicBrowserCache,
		}
		graphqlServer = setupGraphQLEndpoint(graphqlResolver, &configData.Agent.GraphQLConfig, log)
		defer func() {
			if graphqlServer != nil {
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer shutdownCancel()
				if err := graphqlServer.Shutdown(shutdownCtx); err != nil {
					log.Errorf("Failed to shutdown GraphQL server: %v", err)
				}
			}
		}()
	} else {
		log.Info("GraphQL server disabled via configuration")
	}

	if configData.Agent.APIURL != "" && configData.Agent.AuthToken != "" {
		enableBackendConnection(&configData, communicationState, controlLoop, communicationState.Logger)
	} else {
		log.Warnf("No backend connection enabled, please set API_URL and AUTH_TOKEN")
	}

	// Start the system snapshot logger
	go SystemSnapshotLogger(ctx, controlLoop)

	// Start the control loop
	err = controlLoop.Execute(ctx)
	if err != nil {
		log.Errorf("Control loop failed: %w", err)
		sentry.ReportIssuef(sentry.IssueTypeFatal, log, "Control loop failed: %w", err)
	}

	log.Info("umh-core completed")
}

// SystemSnapshotLogger logs the system snapshot every 5 seconds
// It is an example on how to access the system snapshot and log it for communication with other components
func SystemSnapshotLogger(ctx context.Context, controlLoop *control.ControlLoop) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	snap_logger := logger.For("SnapshotLogger")
	if snap_logger == nil {
		snap_logger = zap.NewNop().Sugar()
	}

	snap_logger.Info("Starting system snapshot logger")

	for {
		select {
		case <-ctx.Done():
			snap_logger.Info("Stopping system snapshot logger")
			return
		case <-ticker.C:
			snapshot := controlLoop.GetSystemSnapshot()
			if snapshot == nil {
				sentry.ReportIssuef(sentry.IssueTypeWarning, snap_logger, "[SystemSnapshotLogger] No system snapshot available")
				continue
			}

			snap_logger.Infof("=== System Snapshot (Tick %d) - %d Managers ===",
				snapshot.Tick, len(snapshot.Managers))

			// Log manager information
			for managerName, manager := range snapshot.Managers {
				instances := manager.GetInstances()

				if len(instances) == 0 {
					snap_logger.Infof("📁 %s (tick: %d) - No instances",
						managerName, manager.GetManagerTick())
				} else {
					snap_logger.Infof("📁 %s (tick: %d) - %d instance(s):",
						managerName, manager.GetManagerTick(), len(instances))

					// Log instance information with indentation
					for instanceName, instance := range instances {
						statusReason := ""

						// Extract StatusReason from LastObservedState based on manager type
						if instance.LastObservedState != nil {
							switch managerName {
							case "DataFlowCompManagerCore":
								if dfcSnapshot, ok := instance.LastObservedState.(*dataflowcomponent.DataflowComponentObservedStateSnapshot); ok {
									statusReason = dfcSnapshot.ServiceInfo.StatusReason
								}
							case "ProtocolConverterManagerCore":
								if pcSnapshot, ok := instance.LastObservedState.(*protocolconverter.ProtocolConverterObservedStateSnapshot); ok {
									statusReason = pcSnapshot.ServiceInfo.StatusReason
								}
							}
						}

						// Format state with emojis for better visibility
						stateIcon := "⚠️"
						switch instance.CurrentState {
						case "active":
							stateIcon = "✅"
						case "stopped":
							stateIcon = "⏹️"
						case "idle":
							stateIcon = "💤"
						case "degraded":
							stateIcon = "⚠️"
						}

						if statusReason != "" {
							snap_logger.Infof("  └─ %s %s: %s → %s | %s",
								stateIcon, instanceName, instance.CurrentState, instance.DesiredState, statusReason)
						} else {
							snap_logger.Infof("  └─ %s %s: %s → %s",
								stateIcon, instanceName, instance.CurrentState, instance.DesiredState)
						}
					}
				}
			}
		}
	}
}

func enableBackendConnection(config *config.FullConfig, communicationState *communication_state.CommunicationState, controlLoop *control.ControlLoop, logger *zap.SugaredLogger) {

	logger.Info("Enabling backend connection")
	// directly log the config to console, not to the logger
	if config == nil {
		logger.Warn("Config is nil, cannot enable backend connection")
		return
	}

	if config.Agent.APIURL != "" && config.Agent.AuthToken != "" {
		// This can temporarely deactivated, e.g., during integration tests where just the mgmtcompanion-config is changed directly

		login := v2.NewLogin(config.Agent.AuthToken, config.Agent.AllowInsecureTLS, config.Agent.APIURL, logger)
		if login == nil {
			sentry.ReportIssuef(sentry.IssueTypeError, logger, "[v2.NewLogin] Failed to create login object")
			return
		}
		communicationState.LoginResponseMu.Lock()
		communicationState.LoginResponse = login
		communicationState.LoginResponseMu.Unlock()
		logger.Info("Backend connection enabled, login response: ", zap.Any("login_name", login.Name))

		// Get the config manager from the control loop
		configManager := controlLoop.GetConfigManager()
		snapshotManager := controlLoop.GetSnapshotManager()
		communicationState.InitialiseAndStartPuller()
		communicationState.InitialiseAndStartPusher()
		communicationState.InitialiseAndStartSubscriberHandler(time.Minute*5, time.Minute, config, snapshotManager, configManager)
		communicationState.InitialiseAndStartRouter()
		communicationState.InitialiseReAuthHandler(config.Agent.AuthToken, config.Agent.AllowInsecureTLS)

	}

	logger.Info("Backend connection enabled")
}

// setupGraphQLEndpoint sets up GraphQL endpoint using Gin and proper gqlgen handler
func setupGraphQLEndpoint(resolver *graphql.Resolver, cfg *config.GraphQLConfig, logger *zap.SugaredLogger) *http.Server {
	// Set Gin mode based on debug setting
	if cfg.Debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Add middleware
	router.Use(gin.Recovery())
	router.Use(func(c *gin.Context) {
		// Simple logging middleware
		start := time.Now()
		c.Next()
		if cfg.Debug {
			logger.Infof("GraphQL %s %s %d %v", c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(start))
		}
	})

	// Add CORS middleware if origins are specified
	if len(cfg.CORSOrigins) > 0 {
		router.Use(func(c *gin.Context) {
			origin := c.Request.Header.Get("Origin")
			// Simple CORS - in production, use proper CORS middleware
			for _, allowedOrigin := range cfg.CORSOrigins {
				if allowedOrigin == "*" || allowedOrigin == origin {
					c.Header("Access-Control-Allow-Origin", allowedOrigin)
					c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
					c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
					break
				}
			}
			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatus(204)
				return
			}
			c.Next()
		})
	}

	// Create proper GraphQL handler
	schema := graphql.NewExecutableSchema(graphql.Config{Resolvers: resolver})
	srv := handler.NewDefaultServer(schema)

	// Add error handling and recovery
	srv.SetRecoverFunc(func(ctx context.Context, err interface{}) error {
		sentry.ReportIssue(fmt.Errorf("GraphQL panic: %v", err), sentry.IssueTypeFatal, logger)
		return fmt.Errorf("internal server error")
	})

	// Add GraphQL routes
	router.POST("/graphql", gin.WrapH(srv))
	router.GET("/graphql", gin.WrapH(srv)) // Allow GET for introspection

	// Add GraphiQL playground for development
	if cfg.Debug {
		router.GET("/", gin.WrapH(playground.Handler("GraphQL Playground", "/graphql")))
	}

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: router,
	}

	go func() {
		logger.Infof("Starting GraphQL server on port %d (debug: %v)", cfg.Port, cfg.Debug)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			sentry.ReportIssue(err, sentry.IssueTypeFatal, logger)
		}
	}()

	return server
}
