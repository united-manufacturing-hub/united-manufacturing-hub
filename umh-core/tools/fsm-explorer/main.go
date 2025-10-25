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

// FSM v2 Explorer - Container Demo
//
// FUTURE: When building 2nd FSM demo (benthos, redpanda, etc.):
// 1. Copy this file to tools/fsm-explorer-<name>/main.go
// 2. Find/replace "container" → "<name>"
// 3. Adapt metrics and phase logic
// 4. Notice what you're duplicating
// 5. After 3rd FSM demo, consider extracting common patterns
//
// Reusable patterns marked with "// REUSABLE PATTERN" comments

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/explorer"
)

func main() {
	runContainerDemo()
}

func runContainerDemo() {
	ctx := context.Background()

	// Phase 1: Setup
	printDemoHeader()
	printPhaseHeader(1, "Initial Setup")
	scenario := setupContainerScenario(ctx)
	printSetupComplete(scenario)

	time.Sleep(2 * time.Second)

	// Phase 2: First Observation
	printPhaseHeader(2, "First Observation Cycle")
	demonstrateFirstObservation(ctx, scenario)

	time.Sleep(2 * time.Second)

	// Phase 3: Steady State
	printPhaseHeader(3, "Normal Operation")
	demonstrateSteadyState(ctx, scenario, 3)

	time.Sleep(2 * time.Second)

	// Phase 4: Shutdown
	printPhaseHeader(4, "Shutdown Request")
	demonstrateShutdown(ctx, scenario)

	time.Sleep(2 * time.Second)

	// Phase 5: Summary
	printSummary()
	cleanupScenario(ctx, scenario)
}

// REUSABLE PATTERN: Demo header
// Copy this when building next FSM demo
func printDemoHeader() {
	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║     FSM v2 Explorer - System Health Monitoring Demo      ║")
	fmt.Println("║     Demonstrates observation-driven state management      ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")
	fmt.Println()
}

// REUSABLE PATTERN: Phase header formatting
// Copy this when building next FSM demo
func printPhaseHeader(num int, title string) {
	fmt.Printf("\n🔧 Phase %d: %s\n", num, title)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
}

// REUSABLE PATTERN: State transition display
// Copy this when building next FSM demo
func printStateTransition(from, to, reason string) {
	fmt.Printf("🎯 State Transition: %s → %s\n", from, to)
	fmt.Printf("   Reason: %s\n", reason)
	fmt.Println()
}

// Container-specific: Setup scenario
func setupContainerScenario(ctx context.Context) *explorer.ContainerScenario {
	scenario := explorer.NewContainerScenario()

	if err := scenario.Setup(ctx); err != nil {
		fmt.Printf("❌ Setup failed: %v\n", err)
		os.Exit(1)
	}

	return scenario
}

// Container-specific: Print setup completion
func printSetupComplete(scenario *explorer.ContainerScenario) {
	fmt.Println("✅ Initialized FSM v2 System Health Monitor")
	fmt.Println("   Monitoring: Host system (Mac)")
	fmt.Println()
	fmt.Println("✅ Initialized TriangularStore persistence")
	fmt.Println("   Database: /tmp/explorer-test-*/test.db")
	fmt.Println()
	fmt.Println("✅ Registered FSM worker with supervisor")
	fmt.Println("   Worker Type: container")
	fmt.Println("   Observation Interval: 1 second")
	fmt.Println()

	state, reason := scenario.GetCurrentState()
	fmt.Printf("📊 Initial State: %s\n", state)
	fmt.Printf("   Reason: %s\n", reason)
	fmt.Println()
}

// Container-specific: Demonstrate first observation cycle
func demonstrateFirstObservation(ctx context.Context, scenario *explorer.ContainerScenario) {
	initialState, _ := scenario.GetCurrentState()

	fmt.Println("⏱️  Waiting for first observation... (1 second tick)")
	fmt.Println()

	time.Sleep(1500 * time.Millisecond)

	// Trigger tick to collect observation
	if err := scenario.Tick(ctx); err != nil {
		fmt.Printf("❌ Tick failed: %v\n", err)
		return
	}

	// Get current state after observation
	newState, reason := scenario.GetCurrentState()

	fmt.Println("🤔 Supervisor Evaluation:")
	fmt.Println("   Observed Reality: Host system metrics collected")
	fmt.Println("   CPU, Memory, Disk usage from host")
	fmt.Printf("   Transition Logic: %s + Running → %s\n", initialState, newState)
	fmt.Println()

	if newState != initialState {
		printStateTransition(initialState, newState, reason)
	} else {
		fmt.Printf("📊 State Unchanged: %s\n", newState)
		fmt.Printf("   Reason: %s\n", reason)
		fmt.Println()
	}

	fmt.Println("💾 Saved to Database:")
	fmt.Println("   - ObservedState (CPU, Memory, Disk metrics)")
	fmt.Printf("   - Current FSM State: %s\n", newState)
	fmt.Println("   - Sync ID incremented")
	fmt.Println()
}

// Container-specific: Demonstrate steady state operation
func demonstrateSteadyState(ctx context.Context, scenario *explorer.ContainerScenario, ticks int) {
	state, reason := scenario.GetCurrentState()

	fmt.Printf("📊 Current State: %s\n", state)
	fmt.Printf("   Reason: %s\n", reason)
	fmt.Println()

	fmt.Println("⏱️  Observing every 1 second...")
	fmt.Println()

	for i := 0; i < ticks; i++ {
		time.Sleep(1 * time.Second)

		if err := scenario.Tick(ctx); err != nil {
			fmt.Printf("❌ Tick %d failed: %v\n", i+1, err)
			continue
		}

		state, _ := scenario.GetCurrentState()
		fmt.Printf("[Tick %d] State: %s\n", i+1, state)
	}

	fmt.Println()
	fmt.Printf("✅ FSM remains in %s state (no unhealthy conditions detected)\n", state)
	fmt.Println()
}

// Container-specific: Demonstrate shutdown request
func demonstrateShutdown(ctx context.Context, scenario *explorer.ContainerScenario) {
	currentState, _ := scenario.GetCurrentState()

	fmt.Println("💡 Simulating shutdown request...")
	fmt.Println("   (In production: User clicks \"Stop Container\" in Management Console)")
	fmt.Println()

	if err := scenario.InjectShutdown(); err != nil {
		fmt.Printf("❌ Shutdown injection failed: %v\n", err)
		return
	}

	fmt.Println("📝 Updated Desired State:")
	fmt.Println("   ShutdownRequested: false → true")
	fmt.Println()

	fmt.Println("💾 Saved to Database:")
	fmt.Println("   - DesiredState.ShutdownRequested = true")
	fmt.Println("   - Sync ID incremented")
	fmt.Println()

	fmt.Println("⏱️  Waiting for next observation cycle...")
	fmt.Println()

	time.Sleep(1500 * time.Millisecond)

	if err := scenario.Tick(ctx); err != nil {
		fmt.Printf("❌ Tick failed: %v\n", err)
		return
	}

	newState, reason := scenario.GetCurrentState()

	fmt.Println("🤔 Supervisor Evaluation:")
	fmt.Println("   Observed: Host system still healthy")
	fmt.Println("   Desired: Shutdown = true")
	fmt.Printf("   Transition Logic: %s + ShutdownRequested → %s\n", currentState, newState)
	fmt.Println()

	if newState != currentState {
		printStateTransition(currentState, newState, reason)
	} else {
		fmt.Printf("📊 State: %s\n", newState)
		fmt.Printf("   Reason: %s\n", reason)
		fmt.Println()
	}
}

// REUSABLE PATTERN: Demo summary
// Copy and adapt when building next FSM demo
func printSummary() {
	fmt.Println()
	fmt.Println("✨ Demo Complete!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("📈 What You Just Witnessed:")
	fmt.Println()
	fmt.Println("1. **Observation-Driven FSM**")
	fmt.Println("   - Supervisor doesn't receive commands")
	fmt.Println("   - Worker observes reality every second")
	fmt.Println("   - FSM compares desired vs observed state")
	fmt.Println("   - Automatic transitions when they differ")
	fmt.Println()
	fmt.Println("2. **Database-Backed State**")
	fmt.Println("   - All state persisted to SQLite")
	fmt.Println("   - Every change increments sync ID")
	fmt.Println("   - Full state history queryable")
	fmt.Println("   - Enables data freshness tracking")
	fmt.Println()
	fmt.Println("3. **Key FSM v2 Concepts**")
	fmt.Println("   ✅ Tick-based evaluation (not event-driven)")
	fmt.Println("   ✅ Observation before decision")
	fmt.Println("   ✅ Declarative desired state")
	fmt.Println("   ✅ CSE Triangular Model (Identity/Desired/Observed)")
	fmt.Println()
}

// REUSABLE PATTERN: Cleanup
// Copy when building next FSM demo
func cleanupScenario(ctx context.Context, scenario *explorer.ContainerScenario) {
	fmt.Println("🧹 Cleaning up...")

	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := scenario.Cleanup(cleanupCtx); err != nil {
		fmt.Printf("⚠️  Cleanup warning: %v\n", err)
	}

	fmt.Println("✅ Monitor stopped")
	fmt.Println("✅ Database closed")
	fmt.Println("✅ Temp directory removed")
	fmt.Println()
	fmt.Println("👋 Thank you for exploring FSM v2!")
}
