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

//go:build integration

package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GraphQL Integration Tests", func() {
	var (
		server    *Server
		serverCtx context.Context
		cancel    context.CancelFunc
		port      = 8899 // Use different port for testing
	)

	BeforeEach(func() {
		// Setup test server
		resolver := &Resolver{
			TopicBrowserCache: NewMockTopicBrowserCache(),
		}

		config := &ServerConfig{
			Port:        port,
			Debug:       true,
			CORSOrigins: []string{"*"},
		}

		var err error
		server, err = NewServer(resolver, config, nil)
		Expect(err).NotTo(HaveOccurred())

		// Start server in background
		serverCtx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		go func() {
			defer GinkgoRecover()
			if err := server.Start(serverCtx); err != nil {
				GinkgoWriter.Printf("Server start error: %v\n", err)
			}
		}()

		// Wait for server to start
		time.Sleep(200 * time.Millisecond)
	})

	AfterEach(func() {
		if server != nil {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			server.Stop(shutdownCtx)
		}
		if cancel != nil {
			cancel()
		}
	})

	Describe("GraphQL Queries", func() {
		Context("when querying topics", func() {
			It("should return topics with metadata and events", func() {
				query := `{
					topics(limit: 10) {
						topic
						metadata {
							key
							value
						}
						lastEvent {
							... on TimeSeriesEvent {
								producedAt
								scalarType
								stringValue
							}
						}
					}
				}`

				response := executeGraphQLQuery(query, port)

				Expect(response.Data).NotTo(BeNil())
				Expect(response.Errors).To(BeEmpty())
			})

			It("should handle topic filtering", func() {
				query := `{
					topics(filter: { text: "temperature" }, limit: 5) {
						topic
						metadata {
							key
							value
						}
					}
				}`

				response := executeGraphQLQuery(query, port)

				Expect(response.Data).NotTo(BeNil())
				Expect(response.Errors).To(BeEmpty())
			})
		})

		Context("when querying a specific topic", func() {
			It("should return the topic details", func() {
				query := `{
					topic(topic: "enterprise.site.area.productionLine.workstation.sensor.temperature") {
						topic
						metadata {
							key
							value
						}
					}
				}`

				response := executeGraphQLQuery(query, port)

				Expect(response.Data).NotTo(BeNil())
				Expect(response.Errors).To(BeEmpty())
			})

			It("should handle non-existent topics gracefully", func() {
				query := `{
					topic(topic: "non.existent.topic") {
						topic
						metadata {
							key
							value
						}
					}
				}`

				response := executeGraphQLQuery(query, port)

				// Should not error, just return null
				Expect(response.Errors).To(BeEmpty())
			})
		})
	})

	Describe("HTTP Server Features", func() {
		Context("CORS functionality", func() {
			It("should set CORS headers for OPTIONS requests", func() {
				client := &http.Client{Timeout: 5 * time.Second}

				req, err := http.NewRequest("OPTIONS", fmt.Sprintf("http://localhost:%d/graphql", port), nil)
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("Origin", "http://localhost:3000")

				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				corsHeader := resp.Header.Get("Access-Control-Allow-Origin")
				Expect(corsHeader).NotTo(BeEmpty())
			})
		})

		Context("GraphiQL playground", func() {
			It("should serve playground in debug mode", func() {
				client := &http.Client{Timeout: 5 * time.Second}

				resp, err := client.Get(fmt.Sprintf("http://localhost:%d/", port))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			})
		})

		Context("error handling", func() {
			It("should handle malformed GraphQL queries", func() {
				client := &http.Client{Timeout: 5 * time.Second}

				malformedJSON := `{"query": "{ invalid syntax }`

				req, err := http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/graphql", port),
					bytes.NewBufferString(malformedJSON))
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				// Should return 400 for malformed JSON
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			})

			It("should handle invalid GraphQL syntax gracefully", func() {
				query := `{ invalidField }`

				response := executeGraphQLQueryAllowingErrors(query, port)

				// Should return GraphQL errors for invalid fields
				Expect(response.Errors).NotTo(BeEmpty())
			})
		})
	})
})

type GraphQLResponse struct {
	Data   interface{}    `json:"data"`
	Errors []GraphQLError `json:"errors"`
}

type GraphQLError struct {
	Message string        `json:"message"`
	Path    []interface{} `json:"path"`
}

func executeGraphQLQuery(query string, port int) *GraphQLResponse {
	client := &http.Client{Timeout: 5 * time.Second}

	reqBody := map[string]string{
		"query": query,
	}

	jsonBody, err := json.Marshal(reqBody)
	Expect(err).NotTo(HaveOccurred())

	req, err := http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/graphql", port), bytes.NewBuffer(jsonBody))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()

	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	var response GraphQLResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	Expect(err).NotTo(HaveOccurred())

	return &response
}

func executeGraphQLQueryAllowingErrors(query string, port int) *GraphQLResponse {
	client := &http.Client{Timeout: 5 * time.Second}

	reqBody := map[string]string{
		"query": query,
	}

	jsonBody, err := json.Marshal(reqBody)
	Expect(err).NotTo(HaveOccurred())

	req, err := http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/graphql", port), bytes.NewBuffer(jsonBody))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()

	var response GraphQLResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	Expect(err).NotTo(HaveOccurred())

	return &response
}
