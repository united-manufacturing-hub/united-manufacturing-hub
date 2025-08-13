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

package hwid

import (
	"crypto/rand"
	"os"
	"regexp"

	hash2 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/hash"
	"go.uber.org/zap"
)

func GenerateHWID() string {
	hwidPath := os.Getenv("HWID_PATH")
	if hwidPath == "" {
		hwidPath = "/data/hwid"
	}

	// Try to read the HWID from the file (/data/hwid)
	// If it doesn't exist, generate a new one and write it to the file

	_, err := os.Stat(hwidPath)
	if os.IsNotExist(err) {
		generateNewHWID(hwidPath)
	}

	// Allocate buffer for sha3-512 hash (hex encoded)
	file, err := os.ReadFile(hwidPath) //nolint:gosec // G304: File path from environment variable or default, controlled by deployment
	if err != nil {
		return ""
	}

	// Ensure the HWID is 64 characters long and hex only
	if len(file) != 64 || !regexp.MustCompile(`^[0-9a-fA-F]+$`).Match(file) {
		return ""
	}

	return string(file)
}

func generateNewHWID(hwidPath string) {
	// Generate 1024 bytes of random data and hash it using SHA3-512 and write hex to file
	reader := rand.Reader

	buffer := make([]byte, 1024)

	_, err := reader.Read(buffer)
	if err != nil {
		zap.S().Warnf("Failed to generate HWID: %s", err)

		return
	}

	hash := hash2.Sha3Hash(string(buffer))

	file, err := os.Create(hwidPath) //nolint:gosec // G304: File path from environment variable or default, controlled by deployment for HWID generation
	if err != nil {
		zap.S().Warnf("Failed to create HWID file: %s", err)

		return
	}

	defer func() {
		err := file.Close()
		if err != nil {
			zap.S().Warnf("Failed to close HWID file: %s", err)
		}
	}()

	_, err = file.WriteString(hash)
	if err != nil {
		zap.S().Warnf("Failed to write HWID to file: %s", err)

		return
	}
}
