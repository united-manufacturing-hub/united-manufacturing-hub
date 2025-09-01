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

package graphql

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Server wraps the GraphQL HTTP server with proper setup and lifecycle management.
type Server struct {
	server   *http.Server
	logger   *zap.SugaredLogger
	resolver *Resolver
	config   *ServerConfig
}

// NewServer creates a new GraphQL server with the provided resolver and configuration.
func NewServer(resolver *Resolver, config *ServerConfig, logger *zap.SugaredLogger) (*Server, error) {
	if config == nil {
		config = DefaultServerConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid server configuration: %w", err)
	}

	if logger == nil {
		logger = zap.NewNop().Sugar()
	}

	return &Server{
		resolver: resolver,
		config:   config,
		logger:   logger,
	}, nil
}

// Start starts the GraphQL server with proper middleware and routing setup.
func (s *Server) Start(ctx context.Context) error {
	// Set Gin mode based on debug setting
	if s.config.Debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Add recovery middleware
	router.Use(gin.Recovery())

	// Add logging middleware
	router.Use(s.loggingMiddleware())

	// Add CORS middleware if origins are specified
	if len(s.config.CORSOrigins) > 0 {
		router.Use(s.corsMiddleware())
	}

	// Create GraphQL schema and handler
	schema := NewExecutableSchema(Config{Resolvers: s.resolver})
	srv := handler.New(schema)

	// Add transports (replaces functionality from deprecated NewDefaultServer)
	srv.AddTransport(&transport.Websocket{})
	srv.AddTransport(&transport.Options{})
	srv.AddTransport(&transport.GET{})
	srv.AddTransport(&transport.POST{})
	srv.AddTransport(&transport.MultipartForm{})

	// Add error handling and recovery
	srv.SetRecoverFunc(func(ctx context.Context, err interface{}) error {
		s.logger.Errorf("GraphQL panic: %v", err)

		return errors.New("internal server error")
	})

	// Register GraphQL routes
	router.POST("/graphql", gin.WrapH(srv))
	router.GET("/graphql", gin.WrapH(srv)) // Allow GET for introspection

	// Add GraphiQL playground for development
	if s.config.Debug {
		router.GET("/", gin.WrapH(playground.Handler("GraphQL Playground", "/graphql")))
	}

	// Create HTTP server
	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Infow("Starting GraphQL server",
		"port", s.config.Port,
		"debug", s.config.Debug,
		"cors_origins", s.config.CORSOrigins,
	)

	// Start server in a goroutine
	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.logger.Errorw("GraphQL server failed", "error", err)

	}

	return nil
}

// Stop gracefully stops the GraphQL server.
func (s *Server) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	s.logger.Info("Stopping GraphQL server")

	return s.server.Shutdown(ctx)
}

// loggingMiddleware provides request logging.
func (s *Server) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		if s.config.Debug {
			s.logger.Infow("GraphQL request",
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"status", c.Writer.Status(),
				"duration", time.Since(start),
			)
		}
	}
}

// corsMiddleware provides CORS support.
func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Check if origin is allowed
		for _, allowedOrigin := range s.config.CORSOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				c.Header("Access-Control-Allow-Origin", allowedOrigin)
				c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

				break
			}
		}

		// Handle preflight requests
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(204)

			return
		}

		c.Next()
	}
}
