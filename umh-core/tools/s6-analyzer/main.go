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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

func analyzeService(servicePath string) error {
	ctx := context.Background()
	fsService := filesystem.NewDefaultService()
	s6Service := s6.NewDefaultService()

	fmt.Printf("\n=== Analyzing S6 Service: %s ===\n", servicePath)

	// Check if service directory exists
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		return fmt.Errorf("service directory does not exist: %s", servicePath)
	}

	// Get the actual service name from the path
	serviceName := filepath.Base(servicePath)

	// Analyze main service status using the S6 service
	fmt.Printf("\n[Using S6 Service Status() method]\n")
	status, err := s6Service.Status(ctx, serviceName, fsService)
	if err != nil {
		fmt.Printf("Error getting status via S6 service: %v\n", err)
	} else {
		printServiceInfo(status, "Main Service")
	}

	// Also analyze the raw status file directly for more details
	statusPath := filepath.Join(servicePath, "supervise", "status")
	if err := analyzeRawStatusFile(ctx, statusPath, "Main Service (Raw)", fsService); err != nil {
		fmt.Printf("Main service raw status: %v\n", err)
	}

	// Analyze log service status
	logStatusPath := filepath.Join(servicePath, "log", "supervise", "status")
	if err := analyzeRawStatusFile(ctx, logStatusPath, "Log Service", fsService); err != nil {
		fmt.Printf("Log service status: %v\n", err)
	}

	// Check for down files
	checkDownFile(filepath.Join(servicePath, "down"))
	checkDownFile(filepath.Join(servicePath, "log", "down"))

	// Check run script
	runPath := filepath.Join(servicePath, "run")
	if info, err := os.Stat(runPath); err == nil {
		fmt.Printf("\nRun script:\n")
		fmt.Printf("  Path: %s\n", runPath)
		fmt.Printf("  Permissions: %v\n", info.Mode())
		fmt.Printf("  Size: %d bytes\n", info.Size())
		
		// Show first few lines of run script
		content, err := ioutil.ReadFile(runPath)
		if err == nil {
			lines := strings.Split(string(content), "\n")
			fmt.Printf("  First 5 lines:\n")
			for i := 0; i < len(lines) && i < 5; i++ {
				fmt.Printf("    %s\n", lines[i])
			}
		}
	}

	// Check latest logs
	logPath := filepath.Join(filepath.Dir(filepath.Dir(servicePath)), "logs", filepath.Base(servicePath), "current")
	fmt.Printf("\nLog file: %s\n", logPath)
	if info, err := os.Stat(logPath); err == nil {
		fmt.Printf("  Size: %d bytes\n", info.Size())
		fmt.Printf("  Modified: %v\n", info.ModTime())
		
		// Show last few lines if not empty
		if info.Size() > 0 {
			content, err := ioutil.ReadFile(logPath)
			if err == nil {
				lines := strings.Split(string(content), "\n")
				fmt.Printf("  Last 5 lines:\n")
				start := len(lines) - 6
				if start < 0 {
					start = 0
				}
				for i := start; i < len(lines)-1 && i < start+5; i++ {
					fmt.Printf("    %s\n", lines[i])
				}
			}
		} else {
			fmt.Printf("  ⚠️  Log file is empty!\n")
		}
	} else {
		fmt.Printf("  ⚠️  Log file not found or inaccessible: %v\n", err)
	}

	return nil
}

func printServiceInfo(info s6.ServiceInfo, label string) {
	fmt.Printf("\n%s Status:\n", label)
	fmt.Printf("  Status:       %s\n", info.Status)
	fmt.Printf("  PID:          %d\n", info.Pid)
	fmt.Printf("  PGID:         %d\n", info.Pgid)
	
	if info.Status == s6.ServiceUp {
		fmt.Printf("  Uptime:       %d seconds\n", info.Uptime)
		fmt.Printf("  Ready time:   %d seconds\n", info.ReadyTime)
	} else {
		fmt.Printf("  Exit code:    %d\n", info.ExitCode)
		if info.ExitCode == 111 {
			fmt.Printf("                ⚠️  Exit 111 typically means exec failed (binary not found or not executable)\n")
		}
	}

	fmt.Printf("  Flags:\n")
	fmt.Printf("    Paused:     %v\n", info.IsPaused)
	fmt.Printf("    Finishing:  %v\n", info.IsFinishing)
	fmt.Printf("    Want up:    %v\n", info.IsWantingUp)
	fmt.Printf("    Ready:      %v\n", info.IsReady)
	
	fmt.Printf("  Timestamps:\n")
	fmt.Printf("    Last changed: %v (%v ago)\n", info.LastChangedAt, time.Since(info.LastChangedAt).Round(time.Second))
	fmt.Printf("    Last ready:   %v (%v ago)\n", info.LastReadyAt, time.Since(info.LastReadyAt).Round(time.Second))
}

func analyzeRawStatusFile(ctx context.Context, statusPath string, label string, fsService filesystem.Service) error {
	fmt.Printf("\n%s (%s):\n", label, statusPath)

	// Read the raw status file
	statusData, err := fsService.ReadFile(ctx, statusPath)
	if err != nil {
		return fmt.Errorf("cannot read status file: %w", err)
	}

	// Use the S6 package's parsing function indirectly via reflection or exported function
	// Since parseS6StatusFile is not exported, we'll do basic analysis here
	if len(statusData) != 43 {
		return fmt.Errorf("invalid status file size: got %d bytes, expected 43", len(statusData))
	}

	// Display raw data for debugging
	fmt.Printf("  Raw status (hex): ")
	for i, b := range statusData {
		if i > 0 && i%8 == 0 {
			fmt.Printf("\n                    ")
		}
		fmt.Printf("%02x ", b)
	}
	fmt.Printf("\n")

	// Parse flags byte (offset 42)
	flags := statusData[42]
	fmt.Printf("  Flags byte: 0x%02x\n", flags)
	fmt.Printf("    Paused:     %v (bit 0)\n", (flags&0x01) != 0)
	fmt.Printf("    Finishing:  %v (bit 1)\n", (flags&0x02) != 0)
	fmt.Printf("    Want up:    %v (bit 2)\n", (flags&0x04) != 0)
	fmt.Printf("    Ready:      %v (bit 3)\n", (flags&0x08) != 0)

	// Parse wait status (bytes 40-41)
	wstat := uint16(statusData[40])<<8 | uint16(statusData[41])
	ws := syscall.WaitStatus(wstat)
	fmt.Printf("  Wait status: 0x%04x", wstat)
	switch {
	case ws.Exited():
		fmt.Printf(" (exited with code %d)", ws.ExitStatus())
		if ws.ExitStatus() == 111 {
			fmt.Printf(" ⚠️  Binary not found or not executable")
		}
	case ws.Signaled():
		fmt.Printf(" (killed by signal %d)", ws.Signal())
	}
	fmt.Printf("\n")

	return nil
}

func checkDownFile(path string) {
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("\n⚠️  Down file exists: %s\n", path)
		fmt.Printf("    This prevents S6 from starting the service\n")
	}
}

func main() {
	var servicePath string
	flag.StringVar(&servicePath, "service", "", "Path to S6 service directory (e.g., /data/services/redpanda-redpanda)")
	flag.Parse()

	if servicePath == "" {
		// If no service specified, try to analyze all services in current directory
		if len(flag.Args()) > 0 {
			servicePath = flag.Args()[0]
		} else {
			fmt.Println("Usage: s6-analyzer -service <path-to-service-directory>")
			fmt.Println("   or: s6-analyzer <path-to-service-directory>")
			fmt.Println("\nExample: s6-analyzer /data/services/redpanda-redpanda")
			os.Exit(1)
		}
	}

	if err := analyzeService(servicePath); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}