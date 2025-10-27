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

package http

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/expiremap/v2/pkg/expiremap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/error_handler"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/latency"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/safejson"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

type Endpoint string

var (
	LoginEndpoint Endpoint = "/v2/instance/login"
	PushEndpoint  Endpoint = "/v2/instance/push"
	PullEndpoint  Endpoint = "/v2/instance/pull"
)

const keepAliveTimeout = 90 * time.Second

var secureHTTPClient *http.Client
var insecureHTTPClient *http.Client

func GetClient(insecureTLS bool) *http.Client {
	if !insecureTLS && secureHTTPClient == nil {
		// Create a custom transport with HTTP/2 disabled
		transport := &http.Transport{
			ForceAttemptHTTP2: false,
			TLSNextProto:      make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
			Proxy:             http.ProxyFromEnvironment,
			IdleConnTimeout:   keepAliveTimeout,
		}

		// Create an HTTP client with the custom transport
		secureHTTPClient = &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		}
	}

	if insecureTLS && insecureHTTPClient == nil {
		// Create a custom transport with HTTP/2 disabled and insecure TLS
		transport := &http.Transport{
			ForceAttemptHTTP2: false,
			TLSNextProto:      make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
			Proxy:             http.ProxyFromEnvironment,
			IdleConnTimeout:   keepAliveTimeout,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecureTLS,
				MinVersion:         tls.VersionTLS10, // Allow older TLS versions
			},
		}

		// Create an HTTP client with the custom transport
		insecureHTTPClient = &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		}
	}

	if insecureTLS {
		return insecureHTTPClient
	}

	return secureHTTPClient
}

// LatestExternalIp is the latest external IP address
// Our backend server is configured to set a header containing the client's external IP address
// Note: This is best effort, and may not be accurate if the client is behind a proxy.
var LatestExternalIp net.IP

var latenciesFRB = expiremap.NewEx[time.Time, time.Duration](5*time.Minute, 5*time.Minute)
var latenciesDNS = expiremap.NewEx[time.Time, time.Duration](5*time.Minute, 5*time.Minute)
var latenciesTLS = expiremap.NewEx[time.Time, time.Duration](5*time.Minute, 5*time.Minute)
var latenciesConn = expiremap.NewEx[time.Time, time.Duration](5*time.Minute, 5*time.Minute)
var latenciesXResponseTimeHeader = expiremap.NewEx[time.Time, time.Duration](5*time.Minute, 5*time.Minute)
var latenciesReal = expiremap.NewEx[time.Time, time.Duration](5*time.Minute, 5*time.Minute)

func GetLatencyTimeTillFirstByte() models.Latency {
	return latency.CalculateLatency(latenciesFRB)
}
func GetLatencyTimeTillDNS() models.Latency {
	return latency.CalculateLatency(latenciesDNS)
}
func GetLatencyTimeTillTLS() models.Latency {
	return latency.CalculateLatency(latenciesTLS)
}
func GetLatencyTimeTillConn() models.Latency {
	return latency.CalculateLatency(latenciesConn)
}

func GetLatencyTimeXResponseTimeHeader() models.Latency {
	return latency.CalculateLatency(latenciesXResponseTimeHeader)
}

func GetRealLatency() models.Latency {
	return latency.CalculateLatency(latenciesReal)
}

// setupClientTrace creates and returns an http trace with timing measurements.
func setupClientTrace(requestStart *time.Time, timings *struct {
	firstByte time.Duration
	dns       time.Duration
	tls       time.Duration
	conn      time.Duration
}) *httptrace.ClientTrace {
	var dnsStart, tlsStart, connStart time.Time

	return &httptrace.ClientTrace{
		DNSStart: func(_ httptrace.DNSStartInfo) {
			dnsStart = time.Now()
		},
		DNSDone: func(_ httptrace.DNSDoneInfo) {
			timings.dns = time.Since(dnsStart)
		},
		TLSHandshakeStart: func() {
			tlsStart = time.Now()
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, _ error) {
			timings.tls = time.Since(tlsStart)
		},
		ConnectStart: func(_, _ string) {
			connStart = time.Now()
		},
		ConnectDone: func(_, _ string, _ error) {
			timings.conn = time.Since(connStart)
		},
		GotFirstResponseByte: func() {
			timings.firstByte = time.Since(*requestStart)
		},
	}
}

// processCookies handles cookie updates from response headers.
func processCookies(response *http.Response, cookies *map[string]string) {
	if cookies == nil {
		return
	}

	cookieMap := *cookies
	// Process response cookies
	for _, cookie := range response.Cookies() {
		cookieMap[cookie.Name] = cookie.Value
	}

	// Process Cookie and Set-Cookie headers
	for _, headerName := range []string{"Cookie", "Set-Cookie"} {
		header := response.Header.Get(headerName)
		for _, pair := range strings.Split(header, ";") {
			pair = strings.TrimSpace(pair)

			parts := strings.Split(pair, "=")
			if len(parts) == 2 {
				cookieMap[parts[0]] = parts[1]
			}
		}
	}

	*cookies = cookieMap
}

// processLatencyHeaders handles X-Response-Time header processing and latency calculations.
func processLatencyHeaders(response *http.Response, timeTillFirstByte time.Duration, logger *zap.SugaredLogger) {
	if response == nil {
		return
	}

	xResponseTime := response.Header.Get("X-Response-Time")
	if xResponseTime == "" {
		logger.Warn("X-Response-Time header not found")

		return
	}

	elapsedTime, err := time.ParseDuration(xResponseTime + "ns")
	if err != nil {
		logger.Warnf("Failed to parse X-Response-Time header: %s", xResponseTime)

		return
	}

	now := time.Now()
	latenciesXResponseTimeHeader.Set(now, elapsedTime)
	latenciesReal.Set(now, timeTillFirstByte-elapsedTime)
}

// enhanceConnectionError adds detailed context to common connection errors.
func enhanceConnectionError(err error) error {
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "EOF"):
		return fmt.Errorf("connection closed unexpectedly before receiving response: %w (possible causes: network issues, server timeout, or firewall blocking)", err)
	case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded"):
		return fmt.Errorf("request timed out: %w (possible causes: slow network, server overload, or request too large)", err)
	case strings.Contains(errStr, "connection refused"):
		return fmt.Errorf("connection refused: %w (possible causes: server down, incorrect URL, or firewall blocking)", err)
	}

	return fmt.Errorf("connection error: %w (no response received from server, status code 0)", err)
}

// DoHTTPRequest performs the actual HTTP request and returns the response and any errors
// This is an internal function, better use GetRequest or PostRequest instead.
func DoHTTPRequest(ctx context.Context, url string, header map[string]string, cookies *map[string]string, insecureTLS bool, logger *zap.SugaredLogger) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	// Set headers
	for k, v := range header {
		req.Header.Set(k, v)
	}

	// Set cookies
	if cookies != nil {
		for k, v := range *cookies {
			req.AddCookie(&http.Cookie{Name: k, Value: v})
		}
	}

	// Set long poll headers
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Keep-Alive", fmt.Sprintf("timeout=%d, max=1000", int(keepAliveTimeout.Seconds())))
	req.Header.Set("X-Features", "longpoll;")

	// Setup request tracing
	var (
		requestStart time.Time
		timings      struct{ firstByte, dns, tls, conn time.Duration }
	)

	trace := setupClientTrace(&requestStart, &timings)

	// Send request
	requestStart = time.Now()

	response, err := GetClient(insecureTLS).Do(req.WithContext(httptrace.WithClientTrace(req.Context(), trace)))
	if err != nil {
		if response != nil {
			return response, err
		}
		// Enhance error message for connection failures
		return nil, enhanceConnectionError(err)
	}

	// Record latencies
	now := time.Now()
	latenciesFRB.Set(now, timings.firstByte)
	latenciesDNS.Set(now, timings.dns)
	latenciesTLS.Set(now, timings.tls)
	latenciesConn.Set(now, timings.conn)

	processLatencyHeaders(response, timings.firstByte, logger)

	return response, nil
}

// processJSONResponse processes the HTTP response and unmarshals the JSON body.
func processJSONResponse[R any](response *http.Response, cookies *map[string]string, endpoint Endpoint, logger *zap.SugaredLogger) (*R, int, error) {
	defer func() {
		if err := response.Body.Close(); err != nil {
			// Log error but don't return it since we're in a defer
			// The main error handling will be done by the caller
			logger.Warnf("Failed to close response body: %s", err)
		}
	}()

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, response.StatusCode, err
	}

	if response.StatusCode < 200 || response.StatusCode > 399 {
		// if the error is 401, we want to report it as a login error
		if response.StatusCode == http.StatusUnauthorized {
			// no need to report it to sentry
			return nil, response.StatusCode, errors.New("unauthorized: Authentication failed. Either your API key is invalid or your instance has been removed. Please verify your API key or recreate the instance")
		} else {
			error_handler.ReportHTTPErrors(
				errors.New("error response code: "+response.Status),
				response.StatusCode,
				string(endpoint),
				http.MethodGet,
				nil,
				bodyBytes,
			)
		}

		return nil, response.StatusCode, errors.New("error response code: " + response.Status)
	}

	if len(bodyBytes) == 0 {
		return nil, response.StatusCode, nil
	}

	var typedResult R
	if err := safejson.Unmarshal(bodyBytes, &typedResult); err != nil {
		return nil, response.StatusCode, err
	}

	processCookies(response, cookies)

	// Process client IP
	if ip := net.ParseIP(response.Header.Get("X-Client-Ip")); ip != nil {
		LatestExternalIp = ip
	}

	return &typedResult, response.StatusCode, nil
}

// GetRequest does a GET request to the given endpoint, with optional header and cookies
// It is a wrapper around DoHTTPRequest.
func GetRequest[R any](ctx context.Context, endpoint Endpoint, header map[string]string, cookies *map[string]string, insecureTLS bool, apiURL string, logger *zap.SugaredLogger) (result *R, statusCode int, responseErr error) {
	// Set up context with default 30 second timeout if none provided
	if ctx == nil {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
	}

	url := apiURL + string(endpoint)

	response, err := DoHTTPRequest(ctx, url, header, cookies, insecureTLS, logger)
	if err != nil {
		if response != nil {
			return nil, response.StatusCode, err
		}

		return nil, 0, err
	}

	result, statusCode, responseErr = processJSONResponse[R](response, cookies, endpoint, logger)

	return
}

// DoHTTPPostRequest performs the actual HTTP POST request and returns the response and any errors.
func DoHTTPPostRequest[T any](ctx context.Context, url string, data *T, header map[string]string, cookies *map[string]string, insecureTLS bool, logger *zap.SugaredLogger) (*http.Response, error) {
	// Marshal the data into JSON format
	body, err := safejson.Marshal(data)
	if err != nil {
		return nil, err
	}

	// Create a reader from the body
	bodyReader := bytes.NewReader(body)

	// Create a new HTTP request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bodyReader)
	if err != nil {
		return nil, err
	}

	// Set content type to application/json
	req.Header.Set("Content-Type", "application/json")

	// Add any provided headers
	for k, v := range header {
		req.Header.Set(k, v)
	}

	// Add any provided cookies
	if cookies != nil {
		for k, v := range *cookies {
			req.AddCookie(&http.Cookie{Name: k, Value: v})
		}
	}

	// Remove Content-Length header to enable chunked transfer encoding
	req.ContentLength = -1

	// Enable trace for response time tracking
	var (
		start             time.Time
		timeTillFirstByte time.Duration
	)

	trace := setupClientTrace(&start, &struct {
		firstByte time.Duration
		dns       time.Duration
		tls       time.Duration
		conn      time.Duration
	}{})

	// Send the request
	start = time.Now()

	response, err := GetClient(insecureTLS).Do(req.WithContext(httptrace.WithClientTrace(req.Context(), trace)))
	if err != nil {
		if response != nil {
			return response, err
		}
		// Enhance error message for connection failures
		return nil, enhanceConnectionError(err)
	}

	latenciesFRB.Set(time.Now(), timeTillFirstByte)

	return response, nil
}

// PostRequest does a POST request to the given endpoint, with optional header and cookies
// Note: Cookies will be updated with the response cookies, if not nil
// It is a wrapper around DoHTTPPostRequest.
func PostRequest[R any, T any](ctx context.Context, endpoint Endpoint, data *T, header map[string]string, cookies *map[string]string, insecureTLS bool, apiURL string, logger *zap.SugaredLogger) (result *R, statusCode int, responseErr error) {
	// Set up context with default 30 second timeout if none provided
	if ctx == nil {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
	}

	url := apiURL + string(endpoint)

	response, err := DoHTTPPostRequest(ctx, url, data, header, cookies, insecureTLS, logger)
	if err != nil {
		if response != nil {
			return nil, response.StatusCode, err
		}

		return nil, 0, err
	}

	result, statusCode, responseErr = processJSONResponse[R](response, cookies, endpoint, logger)

	return
}
