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

package watchdog

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/net/context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/fail"
)

/*
# Introduction

	Watchdog is a simple watchdog for goroutines
	To begin using it, create a new Watchdog with NewWatchdog, and start it with Start.
	Afterwards register new goroutines with RegisterHeartbeat.
	Each routine *shall* report it's status regulary using ReportHeartbeatStatus.

## Example
		w := watchdog.NewWatchdog(context.Background(), time.NewTicker(5*time.Second))
		go w.Start()
		uniqueIdentifier := w.RegisterHeartbeat("mygoroutine", 5, 10, true)
		defer w.UnregisterHeartbeat(uniqueIdentifier)
		for {
			// Do something
			w.ReportHeartbeatStatus(uniqueIdentifier, watchdog.HEARTBEAT_STATUS_OK)
		}

## Arguments
	The first argument (name) is used to prevent duplicate registrations.
	The second argument (warningsUntilFailure) is the number of consecutive warnings that can be sent before the watchdog panics the program.
	The third argument (timeout) is the number of seconds after which the watchdog panics the program if no heartbeat was received.
	The fourth argument (onlyIfSubscribers) is a boolean that indicates if the watchdog should only panic if subscribers are present.

## Notes
	You can also use SetHasSubscribers to indicate if the companion has subscribers.
	Outside of tests you should not use SetHasSubscribers, as subscribers.go already handles this.

## Logic
	The watchdog has a ticker to check all registered heartbeats every few seconds.
	For each heartbeat, it checks if the last heartbeat was sent within the timeout.
	If the last heartbeat was sent too long ago, it panics the program.
		If you have configured warningsUntilFailure, it will panic the program if more than warningsUntilFailure warnings are sent for this heartbeat.
		If onlyIfSubscribers is set, it will only panic if subscribers are present.

	If at any point a HEARTBEAT_STATUS_ERROR is sent, it will immediately panic the program, without waiting for the next check.

## Panics
	When the watchdog panics, it will print the name of the goroutine that failed, the unique identifier of the heartbeat.
	If the panic was caused by a timeout, it will also print the file and line where the heartbeat was registered.

## Subscriber
	A subscriber in the context of this library is a user that has registered themselves to the companion (via the PUSH/PULL api), and awaits regular status updates.
	For most goroutines it doesn't make sense to panic if there is no subscriber, as the user is not expecting any status updates.

	In case of the companion the only goroutines not having onlyIfSubscribers set to true are the api ones.


*/

// Watchdog is a simple watchdog for goroutines
type Watchdog struct {
	registeredHeartbeats      map[string]*Heartbeat
	registeredHeartbeatsMutex sync.Mutex
	badHeartbeatChan          chan uuid.UUID
	hasSubscribers            atomic.Bool
	ctx                       context.Context
	ticker                    *time.Ticker
	watchdogID                uuid.UUID
	warningsAreErrors         atomic.Bool
}

// NewWatchdog creates a new Watchdog
func NewWatchdog(ctx context.Context, ticker *time.Ticker, warningsAreErrors bool) *Watchdog {
	w := Watchdog{
		registeredHeartbeats:      make(map[string]*Heartbeat),
		registeredHeartbeatsMutex: sync.Mutex{},
		// badHeartbeatChan is buffered to avoid blocking the watchdog.
		// This might be the case if the watchdog is not started yet and a goroutine is sending a bad heartbeat
		badHeartbeatChan:  make(chan uuid.UUID, 100),
		hasSubscribers:    atomic.Bool{},
		ctx:               ctx,
		ticker:            ticker,
		watchdogID:        uuid.New(),
		warningsAreErrors: atomic.Bool{},
	}
	if warningsAreErrors {
		w.warningsAreErrors.Store(true)
	}
	return &w
}

// Start synchronously starts the watchdog
func (s *Watchdog) Start() {
	for {
		select {
		case uniqueIdentifier := <-s.badHeartbeatChan:
			{
				name := s.getHeartbeatNameByUUID(uniqueIdentifier)
				s.reportStateToNiceFail()
				fail.Fatalf("Heartbeat errored: [%s] %s (%s)", s.watchdogID, name, uniqueIdentifier)
			}
		case <-s.ticker.C:
			{
				s.reportStateToNiceFail()
				now := time.Now()
				zap.S().Infof("Checking heartbeats: [%s] at %s", s.watchdogID, now)
				s.registeredHeartbeatsMutex.Lock()
				for name, hb := range s.registeredHeartbeats {
					lastHeartbeat := now.UTC().Unix() - hb.lastHeatbeatTime.Load()
					if lastHeartbeat < 0 {
						zap.S().Warnf("Time went backwards: [%s] ", s.watchdogID)
					}
					onlyIfHasSub := hb.onlyIfSubscribers
					hasSubs := s.hasSubscribers.Load()
					secondsOverdue := int64(hb.timeout) - lastHeartbeat
					secondsOverdue = secondsOverdue * -1
					// timeout = 0 disables this check
					if secondsOverdue > 0 && hb.timeout != 0 {
						if (onlyIfHasSub && hasSubs) || !onlyIfHasSub {
							// Remove panicked heartbeat from the list
							delete(s.registeredHeartbeats, name)
							s.registeredHeartbeatsMutex.Unlock()
							// zap.S().Debuff("[%s] Heartbeat %s (%s) from %s:%d has failed", s.watchdogID, name, hb.uniqueIdentifier, hb.file, hb.line)
							s.reportStateToNiceFail()
							fail.Fatalf("Heartbeat too old: [%s] %s (%s) [Lifetime heartbeats: %d] (%d seconds overdue)", s.watchdogID, name, hb.uniqueIdentifier, hb.heartbeatsReceived.Load(), secondsOverdue)
						} else {
							zap.S().Infof("Heartbeat: [%s] %s (%s) would fail, but no subscribers are present", s.watchdogID, name, hb.uniqueIdentifier)
						}
					}
				}
				s.registeredHeartbeatsMutex.Unlock()
				zap.S().Infof("Heartbeats are ok: [%s] ", s.watchdogID)
			}
		case <-s.ctx.Done():
			{
				s.reportStateToNiceFail()
				fail.Fatalf("Watchdog context done: [%s] ", s.watchdogID)
			}
		}
	}
}

// HeartbeatStatus is the status of a heartbeat
type HeartbeatStatus int

const (
	// HEARTBEAT_STATUS_OK is the status of a healthy heartbeat
	HEARTBEAT_STATUS_OK HeartbeatStatus = iota
	// HEARTBEAT_STATUS_WARNING is the status of a heartbeat with a warning, given enough warnings, it will panic the program if configured in RegisterHeartbeat
	HEARTBEAT_STATUS_WARNING
	// HEARTBEAT_STATUS_ERROR is the status of a heartbeat with an error, it will panic the program
	HEARTBEAT_STATUS_ERROR
)

// Heartbeat is a heartbeat
type Heartbeat struct {
	uniqueIdentifier     uuid.UUID
	lastReportedStatus   atomic.Int32
	lastHeatbeatTime     atomic.Int64
	file                 string
	line                 int
	warningCount         atomic.Uint32
	warningsUntilFailure uint64
	timeout              uint64
	onlyIfSubscribers    bool
	heartbeatsReceived   atomic.Uint64
}

// RegisterHeartbeat registers a new heartbeat
// It returns the unique identifier of the heartbeat
// Keep that identifier to unregister the heartbeat later
func (s *Watchdog) RegisterHeartbeat(name string, warningsUntilFailure uint64, timeout uint64, onlyIfSubscribers bool) uuid.UUID {
	uniqueIdentifier := uuid.New()
	_, file, line, ok := runtime.Caller(1)

	zap.S().Infof("[%s] Registering heartbeat %s (%s)", s.watchdogID, name, uniqueIdentifier)
	hb := Heartbeat{
		uniqueIdentifier:     uniqueIdentifier,
		warningsUntilFailure: warningsUntilFailure,
		timeout:              timeout,
		onlyIfSubscribers:    onlyIfSubscribers,
	}
	hb.lastHeatbeatTime.Store(time.Now().UTC().Unix())
	if ok {
		hb.file = file
		hb.line = line
	} else {
		zap.S().Warnf("[%s] Unable to get caller file and line for heartbeat %s", s.watchdogID, name)
	}
	s.registeredHeartbeatsMutex.Lock()
	if v, ok := s.registeredHeartbeats[name]; ok {
		s.registeredHeartbeatsMutex.Unlock()
		zap.S().Errorf("[%s] Heartbeat already registered: %s (%s)", s.watchdogID, name, v.uniqueIdentifier)
		s.reportStateToNiceFail()
		fail.Fatalf("Heartbeat already registered: %s", name)
	}
	s.registeredHeartbeats[name] = &hb
	zap.S().Infof("[%s] Registered heartbeat %s (%s)", s.watchdogID, name, uniqueIdentifier)
	s.registeredHeartbeatsMutex.Unlock()
	return uniqueIdentifier
}

// UnregisterHeartbeat unregisters a heartbeat
// Call this when the goroutine is doing a normal exit
func (s *Watchdog) UnregisterHeartbeat(uniqueIdentifier uuid.UUID) {
	zap.S().Infof("[%s] Unregistering heartbeat %s", s.watchdogID, uniqueIdentifier)
	// Find the heartbeat
	name := s.getHeartbeatNameByUUID(uniqueIdentifier)
	if name == "" {
		zap.S().Warnf("[%s] Unregister heartbeat called with unknown identifier: %s", s.watchdogID, uniqueIdentifier)
		return
	}

	s.registeredHeartbeatsMutex.Lock()
	delete(s.registeredHeartbeats, name)
	s.registeredHeartbeatsMutex.Unlock()
	zap.S().Infof("[%s] Unregistered heartbeat %s", s.watchdogID, uniqueIdentifier)
}

// ReportHeartbeatStatus reports the status of a heartbeat
// Call this every time the routine is looping (with HEARTBEAT_STATUS_OK), when it's doing something weird (with HEARTBEAT_STATUS_WARNING) or when it's doing nothing (with HEARTBEAT_STATUS_ERROR)
func (s *Watchdog) ReportHeartbeatStatus(uniqueIdentifier uuid.UUID, status HeartbeatStatus) {
	// Find the heartbeat
	name := s.getHeartbeatNameByUUID(uniqueIdentifier)

	if name == "" {
		fail.WarnBatchedf("Report heartbeat called with unknown identifier: %s", uniqueIdentifier)
		return
	}

	// Update the heartbeat
	s.registeredHeartbeatsMutex.Lock()
	hb := s.registeredHeartbeats[name]
	if hb == nil {
		// If the heartbeat doesn't exist, unlock and return
		s.registeredHeartbeatsMutex.Unlock()
		fail.WarnBatchedf("Report heartbeat called with now invalid name: %s (UUID: %s)", name, uniqueIdentifier)
		return
	}

	hb.lastReportedStatus.Store(int32(status))
	hb.lastHeatbeatTime.Store(time.Now().UTC().Unix())
	hb.heartbeatsReceived.Add(1)
	var warnings uint32
	onlyIfHasSub := hb.onlyIfSubscribers
	hasSubs := s.hasSubscribers.Load()
	if status == HEARTBEAT_STATUS_WARNING {
		warnings = hb.warningCount.Add(1)
		// zap.S().Debuff("[%s] Heartbeat %s (%s) send a warning (%d/%d)", s.watchdogID, name, uniqueIdentifier, warnings, hb.warningsUntilFailure)
		if s.warningsAreErrors.Load() {
			zap.S().Errorf("[%s] Heartbeat %s (%s) send a warning (%d/%d) and warnings are errors", s.watchdogID, name, uniqueIdentifier, warnings, hb.warningsUntilFailure)
			s.badHeartbeatChan <- uniqueIdentifier
		}
	} else if status == HEARTBEAT_STATUS_OK {
		hb.warningCount.Store(0)
	}
	// warningsUntilFailure == 0 disables this check
	if warnings >= uint32(hb.warningsUntilFailure) && hb.warningsUntilFailure != 0 && ((onlyIfHasSub && hasSubs) || !onlyIfHasSub) {
		zap.S().Errorf("[%s] Heartbeat %s (%s) send to many consecutive warnings (%d/%d)", s.watchdogID, name, uniqueIdentifier, warnings, hb.warningsUntilFailure)
		fail.ErrorBatchedf("Heartbeat too many warnings: %s send to many consecutive warnings (%d/%d)", name, warnings, hb.warningsUntilFailure)
		s.badHeartbeatChan <- uniqueIdentifier
	}
	s.registeredHeartbeatsMutex.Unlock()
	if status == HEARTBEAT_STATUS_ERROR {
		zap.S().Errorf("[%s] Heartbeat %s (%s) reported an error", s.watchdogID, name, uniqueIdentifier)
		fail.ErrorBatchedf("Heartbeat reported error: %s", name)
		s.badHeartbeatChan <- uniqueIdentifier
	}
	s.reportStateToNiceFail()
}

// getHeartbeatNameByUUID returns the name of a heartbeat by its unique identifier
func (s *Watchdog) getHeartbeatNameByUUID(uniqueIdentifier uuid.UUID) string {
	// Create a copy of the map while holding the lock
	s.registeredHeartbeatsMutex.Lock()
	heartbeats := make(map[string]*Heartbeat, len(s.registeredHeartbeats))
	for k, v := range s.registeredHeartbeats {
		heartbeats[k] = v
	}
	s.registeredHeartbeatsMutex.Unlock()

	// Search through the copy without holding the lock
	for name, v := range heartbeats {
		if v.uniqueIdentifier == uniqueIdentifier {
			return name
		}
	}
	return ""
}

// SetHasSubscribers sets if the companion has subscribers
func (s *Watchdog) SetHasSubscribers(has bool) {
	s.hasSubscribers.Store(has)
}

func (s *Watchdog) reportStateToNiceFail() {
	// We collect the state of all registered subscribers (name, lastReportedStatus, timeSinceLastReport, warningCount)
	// This is useful for debugging, as it allows us to see the state of all registered subscribers at the time of the panic
	s.registeredHeartbeatsMutex.Lock()
	defer s.registeredHeartbeatsMutex.Unlock()
	for name, hb := range s.registeredHeartbeats {
		if hb == nil {
			// Skip nil heartbeats
			continue
		}

		lastReportedStatus := hb.lastReportedStatus.Load()
		lastHeartbeat := hb.lastHeatbeatTime.Load()
		warningCount := hb.warningCount.Load()

		var status string
		switch lastReportedStatus {
		case int32(HEARTBEAT_STATUS_OK):
			status = "OK"
		case int32(HEARTBEAT_STATUS_WARNING):
			status = "WARNING"
		case int32(HEARTBEAT_STATUS_ERROR):
			status = "ERROR"
		}

		fail.WatchdogReport(name, status, lastHeartbeat, warningCount)
	}
}
