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
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/net/context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
)

/*
# Introduction

	Watchdog is a simple watchdog for goroutines
	To begin using it, create a new Watchdog with NewWatchdog, and start it with Start.
	Afterwards register new goroutines with RegisterHeartbeat.
	Each routine *shall* report it's status regularly using ReportHeartbeatStatus.

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

// Watchdog is a simple watchdog for goroutines.
type Watchdog struct {
	ctx                       context.Context
	registeredHeartbeats      map[string]*Heartbeat
	badHeartbeatChan          chan uuid.UUID
	ticker                    *time.Ticker
	logger                    *zap.SugaredLogger
	registeredHeartbeatsMutex sync.Mutex
	hasSubscribers            atomic.Bool
	warningsAreErrors         atomic.Bool
	watchdogID                uuid.UUID
}

// NewWatchdog creates a new Watchdog.
func NewWatchdog(ctx context.Context, ticker *time.Ticker, warningsAreErrors bool, logger *zap.SugaredLogger) *Watchdog {
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
		logger:            logger,
	}
	if warningsAreErrors {
		w.warningsAreErrors.Store(true)
	}

	return &w
}

// Start synchronously starts the watchdog.
func (s *Watchdog) Start() {
	for {
		select {
		case uniqueIdentifier := <-s.badHeartbeatChan:
			{
				name := s.getHeartbeatNameByUUID(uniqueIdentifier)
				s.reportStateToNiceFail()
				sentry.ReportIssuef(sentry.IssueTypeError, s.logger, "Heartbeat errored: [%s] %s (%s)", s.watchdogID, name, uniqueIdentifier)
				panic(fmt.Sprintf("Heartbeat errored: [%s] %s (%s)", s.watchdogID, name, uniqueIdentifier))
			}
		case <-s.ticker.C:
			{
				s.reportStateToNiceFail()

				now := time.Now()
				s.logger.Debugf("Checking heartbeats: [%s] at %s", s.watchdogID, now)
				s.registeredHeartbeatsMutex.Lock()

				// Check all heartbeats and collect the overdue ones
				var overdueHeartbeat *struct {
					hb             *Heartbeat
					name           string
					secondsOverdue int64
				}

				for name, hb := range s.registeredHeartbeats {
					lastHeartbeat := now.UTC().Unix() - hb.lastHeatbeatTime.Load()
					if lastHeartbeat < 0 {
						s.logger.Warnf("Time went backwards: [%s] ", s.watchdogID)
					}

					onlyIfHasSub := hb.onlyIfSubscribers
					hasSubs := s.hasSubscribers.Load()
					secondsOverdue := int64(hb.timeout) - lastHeartbeat
					secondsOverdue *= -1
					// timeout = 0 disables this check
					if secondsOverdue > 0 && hb.timeout != 0 {
						if (onlyIfHasSub && hasSubs) || !onlyIfHasSub {
							// Found an overdue heartbeat
							overdueHeartbeat = &struct {
								hb             *Heartbeat
								name           string
								secondsOverdue int64
							}{
								name:           name,
								hb:             hb,
								secondsOverdue: secondsOverdue,
							}
							// Remove from the map and break the loop
							delete(s.registeredHeartbeats, name)

							break
						} else {
							s.logger.Infof("Heartbeat: [%s] %s (%s) would fail, but no subscribers are present", s.watchdogID, name, hb.uniqueIdentifier)
						}
					}
				}

				// Unlock before any potential panic
				s.registeredHeartbeatsMutex.Unlock()

				// If we found an overdue heartbeat, try restart or panic
				if overdueHeartbeat != nil {
					logger := s.logger.With("heartbeat", overdueHeartbeat.name)

					// Try restart if function provided
					if overdueHeartbeat.hb.restartFunc != nil {
						logger.Warnf("[Watchdog] Heartbeat failed, attempting restart...")

						if err := overdueHeartbeat.hb.restartFunc(); err != nil {
							// Restart failed → panic with proper UUID format
							sentry.ReportIssuef(sentry.IssueTypeError, logger,
								"Watchdog restart failed: [%s] %s (%s) - %s",
								s.watchdogID, overdueHeartbeat.name, overdueHeartbeat.hb.uniqueIdentifier, err.Error())
							panic(fmt.Sprintf("Watchdog restart failed: [%s] %s (%s) - %s",
								s.watchdogID, overdueHeartbeat.name, overdueHeartbeat.hb.uniqueIdentifier, err.Error()))
						}

						// Restart succeeded → reset counter and re-register
						logger.Infof("[Watchdog] Restart successful, resetting heartbeat")
						s.registeredHeartbeatsMutex.Lock()
						now := time.Now()
						overdueHeartbeat.hb.lastHeatbeatTime.Store(now.UTC().Unix())
						s.registeredHeartbeats[overdueHeartbeat.name] = overdueHeartbeat.hb
						s.registeredHeartbeatsMutex.Unlock()
						continue
					}

					// No restart function → panic (backward compatible)
					errorMsg := fmt.Sprintf("Heartbeat too old: [%s] %s (%s) [Lifetime heartbeats: %d] (%d seconds overdue)",
						s.watchdogID, overdueHeartbeat.name, overdueHeartbeat.hb.uniqueIdentifier,
						overdueHeartbeat.hb.heartbeatsReceived.Load(), overdueHeartbeat.secondsOverdue)

					panic(errorMsg)
				}

				s.logger.Debugf("Heartbeats are ok: [%s] ", s.watchdogID)
			}
		case <-s.ctx.Done():
			{
				s.reportStateToNiceFail()
				sentry.ReportIssuef(sentry.IssueTypeError, s.logger, "Watchdog context done: [%s] ", s.watchdogID)
			}
		}
	}
}

// HeartbeatStatus is the status of a heartbeat.
type HeartbeatStatus int

const (
	// HEARTBEAT_STATUS_OK is the status of a healthy heartbeat.
	HEARTBEAT_STATUS_OK HeartbeatStatus = iota
	// HEARTBEAT_STATUS_WARNING is the status of a heartbeat with a warning, given enough warnings, it will panic the program if configured in RegisterHeartbeat.
	HEARTBEAT_STATUS_WARNING
	// HEARTBEAT_STATUS_ERROR is the status of a heartbeat with an error, it will panic the program.
	HEARTBEAT_STATUS_ERROR
)

// Heartbeat is a heartbeat.
type Heartbeat struct {
	file                 string
	lastHeatbeatTime     atomic.Int64
	line                 int
	warningsUntilFailure uint64
	timeout              uint64
	heartbeatsReceived   atomic.Uint64
	lastReportedStatus   atomic.Int32
	warningCount         atomic.Uint32
	uniqueIdentifier     uuid.UUID
	onlyIfSubscribers    bool
	restartFunc          func() error
}

// RegisterHeartbeatWithRestart registers a heartbeat with optional restart callback.
// If restartFunc is provided, it will be called before panic when heartbeat fails.
// If restart succeeds (returns nil), the heartbeat counter resets and monitoring continues.
// If restart fails or restartFunc is nil, the watchdog panics as before (backward compatible).
func (s *Watchdog) RegisterHeartbeatWithRestart(name string, warningsUntilFailure uint64, timeout uint64, onlyIfSubscribers bool, restartFunc func() error) uuid.UUID {
	uniqueIdentifier := uuid.New()
	_, file, line, ok := runtime.Caller(1)

	s.logger.Infof("[%s] Registering heartbeat %s (%s)", s.watchdogID, name, uniqueIdentifier)
	hb := Heartbeat{
		uniqueIdentifier:     uniqueIdentifier,
		warningsUntilFailure: warningsUntilFailure,
		timeout:              timeout,
		onlyIfSubscribers:    onlyIfSubscribers,
		restartFunc:          restartFunc,
	}
	hb.lastHeatbeatTime.Store(time.Now().UTC().Unix())

	if ok {
		hb.file = file
		hb.line = line
	} else {
		s.logger.Warnf("[%s] Unable to get caller file and line for heartbeat %s", s.watchdogID, name)
	}

	s.registeredHeartbeatsMutex.Lock()

	if v, ok := s.registeredHeartbeats[name]; ok {
		s.registeredHeartbeatsMutex.Unlock()
		s.logger.Errorf("[%s] Heartbeat already registered: %s (%s)", s.watchdogID, name, v.uniqueIdentifier)
		sentry.ReportIssuef(sentry.IssueTypeError, s.logger, "Heartbeat already registered: %s", name)
		panic(fmt.Sprintf("Heartbeat already registered: %s (%s)", name, v.uniqueIdentifier))
	}

	s.registeredHeartbeats[name] = &hb
	s.logger.Infof("[%s] Registered heartbeat %s (%s)", s.watchdogID, name, uniqueIdentifier)
	s.registeredHeartbeatsMutex.Unlock()

	return uniqueIdentifier
}

// RegisterHeartbeat registers a new heartbeat
// It returns the unique identifier of the heartbeat
// Keep that identifier to unregister the heartbeat later.
func (s *Watchdog) RegisterHeartbeat(name string, warningsUntilFailure uint64, timeout uint64, onlyIfSubscribers bool) uuid.UUID {
	return s.RegisterHeartbeatWithRestart(name, warningsUntilFailure, timeout, onlyIfSubscribers, nil)
}

// UnregisterHeartbeat unregisters a heartbeat
// Call this when the goroutine is doing a normal exit.
func (s *Watchdog) UnregisterHeartbeat(uniqueIdentifier uuid.UUID) {
	s.logger.Infof("[%s] Unregistering heartbeat %s", s.watchdogID, uniqueIdentifier)
	// Find the heartbeat
	name := s.getHeartbeatNameByUUID(uniqueIdentifier)
	if name == "" {
		s.logger.Warnf("[%s] Unregister heartbeat called with unknown identifier: %s", s.watchdogID, uniqueIdentifier)

		return
	}

	s.registeredHeartbeatsMutex.Lock()
	delete(s.registeredHeartbeats, name)
	s.registeredHeartbeatsMutex.Unlock()
	s.logger.Infof("[%s] Unregistered heartbeat %s", s.watchdogID, uniqueIdentifier)
}

// ReportHeartbeatStatus reports the status of a heartbeat
// Call this every time the routine is looping (with HEARTBEAT_STATUS_OK), when it's doing something weird (with HEARTBEAT_STATUS_WARNING) or when it's doing nothing (with HEARTBEAT_STATUS_ERROR).
func (s *Watchdog) ReportHeartbeatStatus(uniqueIdentifier uuid.UUID, status HeartbeatStatus) {
	// Find the heartbeat
	name := s.getHeartbeatNameByUUID(uniqueIdentifier)

	if name == "" {
		sentry.ReportIssuef(sentry.IssueTypeError, s.logger, "Report heartbeat called with unknown identifier: %s", uniqueIdentifier)

		return
	}

	// Update the heartbeat
	s.registeredHeartbeatsMutex.Lock()

	hb := s.registeredHeartbeats[name]
	if hb == nil {
		// If the heartbeat doesn't exist, unlock and return
		s.registeredHeartbeatsMutex.Unlock()
		sentry.ReportIssuef(sentry.IssueTypeError, s.logger, "Report heartbeat called with now invalid name: %s (UUID: %s)", name, uniqueIdentifier)

		return
	}

	hb.lastReportedStatus.Store(int32(status))
	hb.lastHeatbeatTime.Store(time.Now().UTC().Unix())
	hb.heartbeatsReceived.Add(1)

	var warnings uint32

	onlyIfHasSub := hb.onlyIfSubscribers
	hasSubs := s.hasSubscribers.Load()

	switch status {
	case HEARTBEAT_STATUS_WARNING:
		warnings = hb.warningCount.Add(1)
		if s.warningsAreErrors.Load() {
			s.logger.Errorf("[%s] Heartbeat %s (%s) send a warning (%d/%d) and warnings are errors", s.watchdogID, name, uniqueIdentifier, warnings, hb.warningsUntilFailure)

			s.badHeartbeatChan <- uniqueIdentifier
		}
	case HEARTBEAT_STATUS_OK:
		hb.warningCount.Store(0)
	case HEARTBEAT_STATUS_ERROR:
		s.logger.Errorf("[%s] Heartbeat %s (%s) reported an error", s.watchdogID, name, uniqueIdentifier)
		sentry.ReportIssuef(sentry.IssueTypeError, s.logger, "Heartbeat reported error: %s", name)

		s.badHeartbeatChan <- uniqueIdentifier
	}
	// warningsUntilFailure == 0 disables this check
	if warnings >= uint32(hb.warningsUntilFailure) && hb.warningsUntilFailure != 0 && ((onlyIfHasSub && hasSubs) || !onlyIfHasSub) {
		s.logger.Errorf("[%s] Heartbeat %s (%s) send to many consecutive warnings (%d/%d)", s.watchdogID, name, uniqueIdentifier, warnings, hb.warningsUntilFailure)
		sentry.ReportIssuef(sentry.IssueTypeError, s.logger, "Heartbeat too many warnings: %s send to many consecutive warnings (%d/%d)", name, warnings, hb.warningsUntilFailure)

		s.badHeartbeatChan <- uniqueIdentifier
	}

	s.registeredHeartbeatsMutex.Unlock()
	s.reportStateToNiceFail()
}

// getHeartbeatNameByUUID returns the name of a heartbeat by its unique identifier.
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

// SetHasSubscribers sets if the companion has subscribers.
func (s *Watchdog) SetHasSubscribers(has bool) {
	s.hasSubscribers.Store(has)
}

func (s *Watchdog) reportStateToNiceFail() {
	// We collect the state of all registered subscribers (name, lastReportedStatus, timeSinceLastReport, warningCount)
	// This is useful for debugging, as it allows us to see the state of all registered subscribers at the time of the panic
	s.registeredHeartbeatsMutex.Lock()
	defer s.registeredHeartbeatsMutex.Unlock()

	/*
		heartbeatReports := make([]string, 0, len(s.registeredHeartbeats))
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
			// Format each heartbeat as a string
			heartbeatReports = append(heartbeatReports, fmt.Sprintf("%s[%s, %d, %d]", name, status, lastHeartbeat, warningCount))
		}

		// Log all heartbeats in a single debug statement if any exist
		if len(heartbeatReports) > 0 {
			s.logger.Debugf("WatchdogReport: %s", strings.Join(heartbeatReports, "; "))
		}
	*/
}
