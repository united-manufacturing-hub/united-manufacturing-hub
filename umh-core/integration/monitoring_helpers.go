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

// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// monitorHealth checks the metrics and golden service.
func monitorHealth() {
	// 1) Check metrics
	checkMetricsHealthy()
	GinkgoWriter.Println("✅ Metrics are healthy")

	// 2) Check Golden service
	checkGoldenServiceWithFailure()
	GinkgoWriter.Println("✅ Golden service is running")

}

func checkMetricsHealthy() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", metricsURL, nil)
	if err != nil {
		Fail(fmt.Errorf("failed to create request: %w\n", err).Error())
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		Fail(fmt.Errorf("failed to get metrics: %w\n", err).Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		Fail(fmt.Sprintf("Metrics endpoint returned non-200: %v", resp.StatusCode))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		Fail(fmt.Errorf("failed to read metrics: %w\n", err).Error())
	}

	checkWhetherMetricsHealthy(string(data))
}

// checkGoldenService sends a test request to the golden service and checks that it returns a 200 status code
func checkGoldenService() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost:8082", bytes.NewBuffer([]byte(`{"message": "test"}`)))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w\n", err)
	}
	req.Header.Set("Content-Type", "application/json")
	checkResp, e := http.DefaultClient.Do(req)
	if e != nil {
		return 0, fmt.Errorf("failed to send request: %w\n", e)
	}
	defer checkResp.Body.Close()

	return checkResp.StatusCode, nil
}

// checkGoldenServiceWithFailure sends a test request to the golden service and fails the test if it doesn't return 200
func checkGoldenServiceWithFailure() {
	statusCode, err := checkGoldenService()
	if err != nil {
		Fail(fmt.Sprintf("failed to check golden service: %v\n", err))
	}
	if statusCode != 200 {
		Fail(fmt.Sprintf("Golden service returned status: %d", statusCode))
	}
}

// checkGoldenServiceStatusOnly sends a test request to the golden service and returns only the status code.
// It handles errors using Expect() so it can be used directly with Eventually().
func checkGoldenServiceStatusOnly() int {
	statusCode, _ := checkGoldenService()
	// We ignore any errors here as they can happen during initial startup
	return statusCode
}

// waitForMetrics polls the /metrics endpoint until it returns 200
func waitForMetrics() error {
	Eventually(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, "GET", metricsURL, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("metrics endpoint returned status %d", resp.StatusCode)
		}
		return nil
	}, 30*time.Second, 1*time.Second).Should(Succeed())
	return nil
}
