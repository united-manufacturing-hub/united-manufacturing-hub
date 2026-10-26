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

// Package managementconsole is the contract between umh-core and the
// Management Console module: a Go module, kept outside this repository, that
// connects an umh-core instance to the Management Console through the relay.
// The relay is the Management Console server that instances connect to.
//
// The package holds only types. umh-core calls a StartFunc once at startup and
// runs the workers in the Result it returns. umh-core's main package chooses
// that start function when it is built, so this package has no state of its
// own.
//
// The package lives in umh-core's fsmv2 module, which the Management Console
// module already imports. The Management Console module depends on fsmv2
// only, never on umh-core's main module, so the dependency between the two
// runs one way.
package managementconsole

import (
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/handoff"
)

// Config holds the instance settings the Management Console module needs.
type Config struct {
	// RelayURL is the address of the Management Console relay.
	RelayURL string
	// AuthToken is the instance's token for logging in to the relay.
	AuthToken string
	// InstanceUUID is the UUID umh-core sends at login. It is a random
	// placeholder for each start; the relay returns the instance's real UUID,
	// which the module reports through Deps.SetInstanceUUID.
	InstanceUUID string
	// AllowInsecureTLS disables certificate checks towards the relay. It is
	// meant for test setups only.
	AllowInsecureTLS bool
	// Location is the instance's location. The key is the level, 0 for the
	// enterprise, then site, area and further levels, and the value is its
	// name.
	Location map[int]string
}

// Deps is what umh-core hands to the start function.
type Deps struct {
	// Inbound takes the messages users sent to this instance, after the
	// Management Console module has accepted them. Content holds the
	// message as the Management Console encodes it for an instance, and
	// SenderEmail the user who sent it. umh-core's message router reads this channel.
	Inbound chan<- handoff.Message
	// Outbound carries the messages umh-core sends to users: status messages
	// and action replies. Content holds the encoded message, and SenderEmail
	// the user it goes to. The Management Console module reads this channel.
	Outbound <-chan handoff.Message
	// SetInstanceUUID tells umh-core the instance UUID the relay returned at
	// login. umh-core uses it in the messages it sends. The Management
	// Console module calls it after each successful login.
	SetInstanceUUID func(uuid string)
	// Config holds the instance settings.
	Config Config
	// Logger is umh-core's logger.
	Logger deps.FSMLogger
}

// Result is what the start function returns.
type Result struct {
	// Children are the workers umh-core runs under its application worker,
	// for example the connection to the relay. Their worker types must be
	// registered with the fsmv2 worker registry.
	Children []config.ChildSpec
	// Status reports the state of the connection for status messages. The
	// Management Console module always sets it. It is nil only in the empty
	// Result of a build without the module.
	Status StatusFunc
}

// StartFunc starts the Management Console module. It returns an error when
// the module cannot run, and umh-core then does not start.
type StartFunc func(Deps) (Result, error)

// StatusFunc returns the state of the connection. umh-core calls it once for
// each status message. It never returns nil. Before the module has observed
// the connection, it returns a Status whose Health is Neutral.
type StatusFunc func(ctx context.Context) *Status

// HealthCategory classifies the connection's health.
type HealthCategory int

const (
	// Neutral means the health is not known yet.
	Neutral HealthCategory = iota
	// Active means the connection works.
	Active
	// Degraded means the connection does not work, or works with errors.
	Degraded
)

// Status is the state of the connection, as shown in the Management Console.
type Status struct {
	// Health classifies the connection, and HealthMessage says why.
	Health        HealthCategory
	HealthMessage string
	// State is the state of the worker that holds the connection, for
	// example "Running".
	State string
	// ConsecutiveErrors is the number of errors since the last success, and
	// LastErrorType names the last one while errors continue.
	ConsecutiveErrors int
	LastErrorType     string
	// OutboundFillPercent is the 95th percentile of how full the queue of
	// messages waiting to go to the relay was over the last 30 seconds, and
	// OutboundPeakPercent the highest value, both from 0 to 100.
	OutboundFillPercent float64
	OutboundPeakPercent float64
	// Push reports the direction from the instance to the relay, the one
	// that carries Outbound, and Pull the direction from the relay to the
	// instance, the one that fills Inbound. Each is nil until it has been
	// observed.
	Push *ChannelStatus
	Pull *ChannelStatus
}

// ChannelStatus is the state of one direction of the connection.
type ChannelStatus struct {
	// State is the worker's state, for example "Running" or "Degraded".
	State string
	// LastErrorType names the last error while errors continue.
	LastErrorType string
	// Messages is the number of messages sent or received, and
	// MessagesDropped the number dropped.
	Messages        int64
	MessagesDropped int64
	// PendingMessages is the number of messages waiting for a retry.
	PendingMessages int
	// ConsecutiveErrors is the number of failures since the last success.
	ConsecutiveErrors int
	// LastStatusCode is the HTTP status code of the last request.
	LastStatusCode int
	// LastLatencyMs is the duration of the last request, in milliseconds.
	LastLatencyMs float64
	// Backpressured reports whether the direction from the relay to the
	// instance is pausing because Inbound is full. Only Pull sets it.
	Backpressured bool
}
