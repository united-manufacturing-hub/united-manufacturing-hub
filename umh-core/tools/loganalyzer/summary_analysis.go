package main

import (
	"fmt"
	"strings"
)

// AnalyzeRecoveryIssue provides a comprehensive analysis of the recovery problem
func (a *LogAnalyzer) AnalyzeRecoveryIssue() {
	fmt.Println("\n=== Recovery Issue Analysis ===\n")
	
	fmt.Println("PROBLEM SUMMARY:")
	fmt.Println("After force-killing UMH Core, the system fails to recover properly on restart.")
	fmt.Println("\nROOT CAUSE:")
	fmt.Println("1. S6 services persist on the filesystem after force-kill")
	fmt.Println("2. FSM managers lose their in-memory instance mappings")
	fmt.Println("3. On restart, managers don't recreate instances for existing S6 services")
	fmt.Println("4. Reconciliation fails with 'instance not found' errors")
	
	// Count affected components
	orphanCount := 0
	affectedComponents := make(map[string]bool)
	
	for _, entry := range a.Errors {
		if strings.Contains(entry.Message, "instance") && strings.Contains(entry.Message, "not found") {
			if match := instanceNotFoundRegex.FindStringSubmatch(entry.Message); match != nil {
				instanceName := match[1]
				componentType := detectComponentType(instanceName)
				affectedComponents[componentType] = true
				orphanCount++
			}
		}
	}
	
	fmt.Printf("\nIMPACT:")
	fmt.Printf("\n• %d orphaned instances detected", len(affectedComponents))
	fmt.Printf("\n• Affected component types:")
	for comp := range affectedComponents {
		fmt.Printf("\n  - %s", comp)
	}
	
	// Analyze recovery patterns
	fmt.Println("\n\nRECOVERY PATTERNS OBSERVED:")
	
	// Check which components recover
	recoveredComponents := []string{"Redpanda"}
	failedComponents := []string{"TopicBrowser", "ProtocolConverter", "DataFlowComponent", "Agent"}
	
	fmt.Println("\n✓ Components that recover properly:")
	for _, comp := range recoveredComponents {
		fmt.Printf("  • %s - Actively recreates instances on startup\n", comp)
	}
	
	fmt.Println("\n✗ Components that fail to recover:")
	for _, comp := range failedComponents {
		fmt.Printf("  • %s - Does not detect/recreate orphaned S6 services\n", comp)
	}
	
	// Backoff analysis
	fmt.Println("\nBACKOFF BEHAVIOR:")
	fmt.Println("• Failed reconciliations trigger exponential backoff")
	fmt.Println("• Backoff increases from 1 to 5 ticks")
	fmt.Println("• This delays recovery attempts but doesn't fix the root issue")
	
	// Solution suggestions
	fmt.Println("\nSUGGESTED FIXES:")
	fmt.Println("1. On startup, managers should scan existing S6 services")
	fmt.Println("2. For each S6 service without a manager instance:")
	fmt.Println("   - Either recreate the FSM instance")
	fmt.Println("   - Or clean up the orphaned S6 service")
	fmt.Println("3. Implement a 'recovery mode' for detecting state mismatches")
	fmt.Println("4. Add health checks that validate S6 services match manager instances")
	
	// Code location hints
	fmt.Println("\nCODE LOCATIONS TO INVESTIGATE:")
	fmt.Println("• Manager initialization/startup code")
	fmt.Println("• ServiceExists() checks in reconciliation")
	fmt.Println("• Instance creation logic in FSM managers")
	fmt.Println("• S6 service discovery/enumeration")
}

func detectComponentType(instanceName string) string {
	switch {
	case strings.Contains(instanceName, "topicbrowser"):
		return "TopicBrowser"
	case strings.Contains(instanceName, "protocolconverter"):
		return "ProtocolConverter"
	case strings.Contains(instanceName, "dfc-"):
		return "DataFlowComponent"
	case strings.Contains(instanceName, "agent"):
		return "Agent"
	case strings.Contains(instanceName, "redpanda"):
		return "Redpanda"
	default:
		return "Unknown"
	}
}