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

package actions

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Ullaakut/nmap/v3"
	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools"
	"go.uber.org/zap"
)

type TestNetworkConnectionAction struct {
	userEmail       string
	actionUUID      uuid.UUID
	instanceUUID    uuid.UUID
	outboundChannel chan *models.UMHMessage
	Payload         models.TestNetworkConnectionPayload
}

// getUserEmail returns the user email of the action
func (t *TestNetworkConnectionAction) getUserEmail() string {
	return t.userEmail
}

// getUuid returns the UUID of the action
func (t *TestNetworkConnectionAction) getUuid() uuid.UUID {
	return t.actionUUID
}

// Parse parses the payload into the TestNetworkConnectionPayload type.
func (t *TestNetworkConnectionAction) Parse(actionPayload interface{}) (err error) {
	t.Payload, err = ParseActionPayload[models.TestNetworkConnectionPayload](actionPayload)
	return err
}

// Validate checks that IP, port, and type are valid.
func (t *TestNetworkConnectionAction) Validate() error {
	// Validate IP address
	if t.Payload.IP == "" {
		return fmt.Errorf("IP address cannot be empty")
	}
	_, err := url.Parse(t.Payload.IP)
	if err != nil {
		return fmt.Errorf("invalid IP address: %s", err.Error())
	}

	// Validate port
	if t.Payload.Port < 1 || t.Payload.Port > 65535 {
		return fmt.Errorf("invalid port: %d", t.Payload.Port)
	}

	// Validate connection type
	if t.Payload.Type != models.OpcuaServer && t.Payload.Type != models.GenericAsset && t.Payload.Type != models.ExternalMQTT {
		return fmt.Errorf("unsupported connection type: %s", t.Payload.Type)
	}

	return nil
}

// executionLock prevents nmap from running in parallel
var executionLock sync.Mutex

// Execute executes the nmap command and returns the scan result as a string.
func (t *TestNetworkConnectionAction) Execute() (scanResult interface{}, actionContext map[string]interface{}, err error) {
	if !SendActionReply(t.instanceUUID, t.userEmail, t.actionUUID, models.ActionExecuting, "Waiting for network scan to start...", t.outboundChannel, models.TestNetworkConnection) {
		return scanResult, nil, fmt.Errorf("error sending action reply")
	}

	executionLock.Lock()
	defer executionLock.Unlock()

	if !SendActionReply(t.instanceUUID, t.userEmail, t.actionUUID, models.ActionExecuting, "Initiating network scan. Creating nmap scanner.", t.outboundChannel, models.TestNetworkConnection) {
		return scanResult, nil, fmt.Errorf("error sending action reply")
	}

	for i := 0; i < 3; i++ {
		scanResult, err = t.scan()
		if err == nil {
			return scanResult, nil, nil
		}
		if !SendActionReply(t.instanceUUID, t.userEmail, t.actionUUID, models.ActionExecuting, fmt.Sprintf("Failed to scan network: %s. Retrying... (%d/3)", err.Error(), i), t.outboundChannel, models.TestNetworkConnection) {
			return scanResult, nil, fmt.Errorf("error sending action reply")
		}
		time.Sleep(5 * time.Second)
	}

	return scanResult, nil, err
}

func (t *TestNetworkConnectionAction) scan() (scanResult interface{}, err error) {
	ctx, cancel := tools.GetXDurationContext(30 * time.Second)
	defer cancel()
	var errorMessage string

	var scanner *nmap.Scanner
	if strings.HasPrefix(t.Payload.IP, "united-manufacturing-hub") {
		// Check if the IP already has svc cluster local suffix
		var ip string
		if strings.HasSuffix(t.Payload.IP, ".svc.cluster.local") {
			ip = t.Payload.IP
		} else {
			ip = fmt.Sprintf("%s.united-manufacturing-hub.svc.cluster.local", t.Payload.IP)
		}

		scanner, err = nmap.NewScanner(
			ctx,
			nmap.WithTargets(ip),
			nmap.WithPorts(strconv.FormatUint(uint64(t.Payload.Port), 10)),
			nmap.WithTraceRoute(),
			nmap.WithSkipHostDiscovery(),
			nmap.WithDebugging(10),
		)
	} else {
		scanner, err = nmap.NewScanner(
			ctx,
			nmap.WithTargets(t.Payload.IP),
			nmap.WithPorts(strconv.FormatUint(uint64(t.Payload.Port), 10)),
			nmap.WithSYNScan(),
			nmap.WithTraceRoute(),
			nmap.WithDebugging(10),
		)
	}
	if err != nil {
		errorMessage = fmt.Sprintf("Failed to create nmap scanner: %s", err.Error())
		zap.S().Errorf(errorMessage)
		return scanResult, fmt.Errorf("%s", errorMessage)
	}

	if !SendActionReply(t.instanceUUID, t.userEmail, t.actionUUID, models.ActionExecuting, "Network scan in progress (Step 1/3). Please wait...", t.outboundChannel, models.TestNetworkConnection) {
		return scanResult, fmt.Errorf("error sending action reply")
	}
	result, warnings, err := scanner.Run()
	if err != nil {
		if warnings != nil {
			zap.S().Errorf("Failed to run nmap scan: (Result: %+v, Warnings: %+v, Error: %v)", result, *warnings, err)
		} else {
			zap.S().Errorf("Failed to run nmap scan: (Result: %+v, Error: %v)", result, err)
		}
		err = fmt.Errorf("error executing nmap command: %s", err.Error())
		return
	}
	if warnings != nil && len(*warnings) > 0 {
		zap.S().Warnf("Nmap returned warnings: \n %v", *warnings)
	}

	if !SendActionReply(t.instanceUUID, t.userEmail, t.actionUUID, models.ActionExecuting, "Network scan in progress (Step 2/3). Please wait...", t.outboundChannel, models.TestNetworkConnection) {
		return scanResult, fmt.Errorf("error sending action reply")
	}

	scanResult = generateOutput(result)
	// zap.S().Debugf("Raw nmap scan result: \n %v", result)
	// zap.S().Debugf("Nmap scan result: \n %s", scanResult)

	if !SendActionReply(t.instanceUUID, t.userEmail, t.actionUUID, models.ActionExecuting, "Network scan in progress (Step 3/3). Please wait...", t.outboundChannel, models.TestNetworkConnection) {
		return scanResult, fmt.Errorf("error sending action reply")
	}

	if result.Stats.Hosts.Up == 0 || len(result.Hosts) == 0 {
		errorMessage = "Nmap result error: no hosts found"
		zap.S().Errorf(errorMessage)
		return scanResult, fmt.Errorf("%s", errorMessage)
	}
	if len(result.Hosts[0].Ports) == 0 {
		errorMessage = "Nmap result error: no ports found"
		zap.S().Errorf(errorMessage)
		return scanResult, fmt.Errorf("%s", errorMessage)
	}
	if result.Hosts[0].Ports[0].Status() == nmap.Closed {
		errorMessage = "Nmap result error: port is closed"
		zap.S().Errorf(errorMessage)
		return scanResult, fmt.Errorf("%s", errorMessage)
	}
	if result.Hosts[0].Ports[0].Status() == nmap.Filtered {
		errorMessage = "Nmap result error: port is filtered"
		zap.S().Errorf(errorMessage)
		return scanResult, fmt.Errorf("%s", errorMessage)
	}
	return scanResult, nil
}

// generateOutput formats the nmap result into a string.
func generateOutput(result *nmap.Run) string {
	var sb strings.Builder
	sb.WriteString(result.Stats.Finished.Summary)
	sb.WriteString("\nCommand: ")
	sb.WriteString(result.Args)
	sb.WriteString("\n\n")
	if len(result.Hosts) > 0 {
		host := result.Hosts[0]
		address := ""
		if len(host.Addresses) > 0 {
			address = host.Addresses[0].String()
		}
		sb.WriteString(fmt.Sprintf("Host: %s\n\n", address))

		// Format the port information
		if len(host.Ports) > 0 {
			// sb.WriteString(fmt.Sprintf("%-9s %-8s %s\n", "PORT", "STATE", "SERVICE"))
			sb.WriteString("PORT\t  STATE\t   SERVICE\n")
			for _, port := range host.Ports {
				sb.WriteString(fmt.Sprintf("%-10s%-9s%-9s\n", fmt.Sprintf("%d/%s", port.ID, port.Protocol), port.State, port.Service))
			}
			sb.WriteString("\n")
		}

		// Format the traceroute information
		if len(host.Trace.Hops) > 0 {
			sb.WriteString("TRACEROUTE\n")
			sb.WriteString("HOP\tRTT\t\t\tADDRESS\n")
			for i, hop := range host.Trace.Hops {
				sb.WriteString(fmt.Sprintf("%-4d%-11s %s\n", i+1, fmt.Sprintf("%s ms", hop.RTT), hop.IPAddr))
			}
		}
	}

	return sb.String()

	// This is the formatted output:

	// Nmap done at Thu Jul 27 15:43:38 2023; 1 IP address (1 host up) scanned in 0.71 seconds
	// Command: /usr/bin/nmap -p 49152 -sS --traceroute -oN - -oX - 192.168.178.24
	//
	// Host: 192.168.1.100
	//
	// PORT			STATE	SERVICE
	// 49152/tcp	open	unknown
	//
	// TRACEROUTE
	// HOP	RTT			ADDRESS
	// 1	102.12 ms	192.168.178.24
}
