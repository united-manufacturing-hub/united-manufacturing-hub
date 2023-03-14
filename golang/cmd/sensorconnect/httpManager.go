// Copyright 2023 UMH Systems GmbH
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
	"go.uber.org/zap"
	"net"
	"net/http"
	"sync"
	"time"
)

var cP sync.Map

func GetHTTPClient(url string) (client http.Client) {
	rawClient, _ := cP.LoadOrStore(
		url, http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				ForceAttemptHTTP2:     false,
				MaxIdleConns:          100,
				MaxConnsPerHost:       0,
				IdleConnTimeout:       10 * time.Second,
				TLSHandshakeTimeout:   1 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		})

	var ok bool
	client, ok = rawClient.(http.Client)
	if !ok {
		zap.S().Fatal("Failed to cast http.Client")
	}
	return
}
