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

package permission_validator

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Role represents the role of a certificate holder.
type Role string

const (
	// RoleAdmin represents an administrator role.
	RoleAdmin Role = "Admin"

	// RoleViewer represents a viewer role.
	RoleViewer Role = "Viewer"

	// RoleEditor represents an editor role.
	RoleEditor Role = "Editor"
)

func EncodeX509Certificate(cert *x509.Certificate) string {
	return base64.StdEncoding.EncodeToString(cert.Raw)
}

func DecodeX509Certificate(encoded string) (*x509.Certificate, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to decode base64"))
	}
	return x509.ParseCertificate(decoded)
}

func GetRoleForLocation(cert *x509.Certificate, location string) (Role, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return GetRoleForLocationWithContext(ctx, cert, location)
}

func GetRoleForLocationWithContext(ctx context.Context, cert *x509.Certificate, location string) (Role, error) {
	if cert == nil {
		return "", errors.New("certificate cannot be nil")
	}
	if location == "" {
		return "", errors.New("location cannot be empty")
	}

	// encode the cert to a string x509(encode)
	certString := EncodeX509Certificate(cert)

	if certString == "" {
		return "", errors.New("failed to encode certificate")
	}

	// Determine the binary name based on OS and architecture
	binaryName := fmt.Sprintf("crypto-core-%s-%s", runtime.GOOS, runtime.GOARCH)

	// Get the absolute path to the binary (assumes it's in the bin subdirectory of this package)
	executableDir, err := filepath.Abs(filepath.Dir(""))
	if err != nil {
		return "", fmt.Errorf("failed to get executable directory: %w", err)
	}
	binaryPath := filepath.Join(executableDir, "bin", binaryName)

	// call the cryptocore GetRoleForLocation via CLI
	cmd := exec.CommandContext(ctx, binaryPath, certString, location)

	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", errors.New("crypto-core execution timed out")
		}
		return "", fmt.Errorf("failed to get role for location: %w", err)
	}

	// Trim whitespace from output
	role := strings.TrimSpace(string(output))
	if role == "" {
		return "", errors.New("crypto-core returned empty role")
	}

	// validate the role
	if role != string(RoleAdmin) && role != string(RoleViewer) && role != string(RoleEditor) {
		return "", errors.New("invalid role: " + role)
	}

	return Role(role), nil
}
