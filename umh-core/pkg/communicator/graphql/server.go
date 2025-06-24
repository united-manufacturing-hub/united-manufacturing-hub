package graphql

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"go.uber.org/zap"
)

// Server wraps the GraphQL HTTP server
type Server struct {
	server   *http.Server
	logger   *zap.Logger
	resolver *Resolver
}

// NewServer creates a new GraphQL server
func NewServer(resolver *Resolver, logger *zap.Logger) *Server {
	return &Server{
		resolver: resolver,
		logger:   logger,
	}
}

// Start starts the GraphQL server on the specified port
func (s *Server) Start(port int) error {
	// Create GraphQL schema
	schema := NewExecutableSchema(Config{Resolvers: s.resolver})

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", s.graphqlHandler(schema))
	mux.HandleFunc("/", s.playgroundHandler())

	// Create HTTP server
	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("Starting GraphQL server", zap.Int("port", port))

	// Start server
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start GraphQL server: %w", err)
	}

	return nil
}

// Simple GraphQL handler (simplified without full gqlgen handler package)
func (s *Server) graphqlHandler(schema graphql.ExecutableSchema) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte(`{"error": "only POST method allowed"}`))
			return
		}

		// For now, return a simple response
		// This will be replaced with proper GraphQL execution
		w.Write([]byte(`{"data": {"message": "GraphQL endpoint working"}}`))
	}
}

// Simple playground handler
func (s *Server) playgroundHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`
<!DOCTYPE html>
<html>
<head>
    <title>GraphQL Playground</title>
</head>
<body>
    <h1>GraphQL Playground</h1>
    <p>GraphQL endpoint is available at <a href="/graphql">/graphql</a></p>
    <p>Send POST requests to /graphql with GraphQL queries.</p>
</body>
</html>
`))
	}
}

// Stop gracefully stops the GraphQL server
func (s *Server) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	s.logger.Info("Stopping GraphQL server")
	return s.server.Shutdown(ctx)
}
