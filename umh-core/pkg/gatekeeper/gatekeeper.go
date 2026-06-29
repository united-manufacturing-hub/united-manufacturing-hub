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

// Package gatekeeper provides security middleware between FSMv2 transport and
// the Router, handling protocol detection, encryption, compression, and permission validation.
package gatekeeper

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/certificatehandler"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/compression"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/protocol"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/validator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
)

// Gatekeeper sits between FSMv2 transport and Router, handling protocol detection,
// encryption, compression, and permission validation.
type Gatekeeper struct {
	// Sub-components
	protocolDetector protocol.Detector
	compression      compression.Handler
	validator        validator.Validator
	certHandler      certificatehandler.Handler

	// Raw channels (received, not owned)
	inboundChan  chan *types.UMHMessage
	outboundChan chan *types.UMHMessage

	// Verified channels (created and owned by gatekeeper)
	verifiedInbound  chan *types.MessageWithSender
	verifiedOutbound chan *types.MessageWithSender

	// TODO(ENG-4558): Remove once actions write MessageWithSender directly.
	legacyOutbound chan *models.UMHMessage

	// Runtime
	logger       *zap.SugaredLogger
	cancel       context.CancelFunc
	instanceUUID string
	locationPath string
	wg           sync.WaitGroup

	// Config
	verifiedInboundSize  int
	verifiedOutboundSize int

	mu      sync.RWMutex
	running bool
}

// New creates a new Gatekeeper.
func New(
	inbound chan *types.UMHMessage,
	outbound chan *types.UMHMessage,
	certHandler certificatehandler.Handler,
	v validator.Validator,
	logger *zap.SugaredLogger,
	opts ...Option,
) *Gatekeeper {
	g := &Gatekeeper{
		inboundChan:          inbound,
		outboundChan:         outbound,
		certHandler:          certHandler,
		validator:            v,
		protocolDetector:     protocol.NewDetector(logger),
		compression:          compression.NewHandler(logger),
		verifiedInboundSize:  DefaultVerifiedInboundBufferSize,
		verifiedOutboundSize: DefaultVerifiedOutboundBufferSize,
		logger:               logger,
	}
	for _, opt := range opts {
		opt(g)
	}
	g.verifiedInbound = make(chan *types.MessageWithSender, g.verifiedInboundSize)
	g.verifiedOutbound = make(chan *types.MessageWithSender, g.verifiedOutboundSize)
	g.legacyOutbound = make(chan *models.UMHMessage, g.verifiedOutboundSize)
	return g
}

// Start begins processing messages.
func (g *Gatekeeper) Start(ctx context.Context) {
	g.mu.Lock()
	if g.running {
		g.mu.Unlock()
		return
	}
	g.running = true
	ctx, g.cancel = context.WithCancel(ctx)
	g.mu.Unlock()

	g.wg.Add(3)
	go g.processInbound(ctx)
	go g.processOutbound(ctx)
	go g.processLegacyOutbound(ctx)
}

// Stop gracefully shuts down the Gatekeeper.
func (g *Gatekeeper) Stop() {
	g.mu.Lock()
	if !g.running {
		g.mu.Unlock()
		return
	}
	g.running = false
	if g.cancel != nil {
		g.cancel()
	}
	g.mu.Unlock()

	g.wg.Wait()
}

// VerifiedInboundChan returns the channel for the Router to read verified messages.
func (g *Gatekeeper) VerifiedInboundChan() <-chan *types.MessageWithSender {
	return g.verifiedInbound
}

// VerifiedOutboundChan returns the channel for the Router to write messages to send.
func (g *Gatekeeper) VerifiedOutboundChan() chan<- *types.MessageWithSender {
	return g.verifiedOutbound
}

// LegacyOutboundChan returns the channel for actions to write pre-encoded models.UMHMessage.
// TODO(ENG-4558): Remove once actions write MessageWithSender directly.
func (g *Gatekeeper) LegacyOutboundChan() chan *models.UMHMessage {
	return g.legacyOutbound
}

// GetChannels implements communicator.ChannelProvider.
func (g *Gatekeeper) GetChannels(_ string) (chan<- *types.UMHMessage, <-chan *types.UMHMessage) {
	return g.inboundChan, g.outboundChan
}

// GetInboundStats implements communicator.ChannelProvider.
func (g *Gatekeeper) GetInboundStats(_ string) (capacity int, length int) {
	return cap(g.inboundChan), len(g.inboundChan)
}

// SetInstanceUUID updates the instance UUID on the gatekeeper and cert handler.
func (g *Gatekeeper) SetInstanceUUID(uuid string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.instanceUUID = uuid

	ch, ok := g.certHandler.(*certificatehandler.CertHandler)
	if ok {
		ch.SetInstanceUUID(uuid)
	}
}

// SetJWT updates the JWT on the certificate handler.
func (g *Gatekeeper) SetJWT(jwt string) {
	ch, ok := g.certHandler.(*certificatehandler.CertHandler)
	if ok {
		ch.SetJWT(jwt)
	}
}

// CertificateHandler returns the certificate handler.
func (g *Gatekeeper) CertificateHandler() certificatehandler.Handler {
	return g.certHandler
}

func (g *Gatekeeper) processInbound(ctx context.Context) {
	defer g.wg.Done()
	defer func() {
		r := recover()
		if r != nil {
			g.logger.Errorw("panic in processInbound", "panic", r)
			sentry.ReportIssuefWithContext(sentry.IssueTypeError, g.logger, map[string]interface{}{"feature": string(deps.FeatureGatekeeper)}, "gatekeeper processInbound panic: %v", r)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-g.inboundChan:
			if !ok {
				return
			}
			g.handleInbound(ctx, msg)
		}
	}
}

func (g *Gatekeeper) handleInbound(ctx context.Context, msg *types.UMHMessage) {
	cryptHandler := g.protocolDetector.Detect(msg)

	decrypted, err := cryptHandler.Decrypt([]byte(msg.Content), msg.Email)
	if err != nil {
		g.logger.Warnw("Failed to decrypt message", "email", msg.Email, "error", err)
		return
	}

	messageContent, err := g.compression.Decode(string(decrypted))
	if err != nil {
		g.logger.Warnw("Failed to decode message", "email", msg.Email, "error", err)
		return
	}

	// Subscribe bypasses cert validation to bootstrap the cert fetcher flow.
	isSubscribe := messageContent.MessageType == models.Subscribe
	if !isSubscribe {
		cert := g.certHandler.Certificate(msg.Email)
		if cert == nil {
			// Proactively fetch cert on first encounter instead of dropping.
			g.logger.Infow("No certificate cached, fetching on demand", "email", msg.Email)
			fetchErr := g.certHandler.FetchCertForEmail(ctx, msg.Email)
			if fetchErr != nil {
				g.logger.Warnw("On-demand cert fetch failed, dropping message", "email", msg.Email, "error", fetchErr)
				return
			}
			cert = g.certHandler.Certificate(msg.Email)
			if cert == nil {
				g.logger.Warnw("Cert still nil after fetch, dropping message", "email", msg.Email)
				return
			}
		}

		rootCA := g.certHandler.RootCA()
		if rootCA == nil {
			g.logger.Warnw("Root CA not yet available, dropping message", "email", msg.Email)
			sentry.ReportIssuefWithContext(sentry.IssueTypeWarning, g.logger, map[string]interface{}{"feature": string(deps.FeatureGatekeeper)}, "gatekeeper: Root CA not yet available, dropping message for %s", msg.Email)
			return
		}
		intermediates := g.certHandler.IntermediateCerts(msg.Email)

		actionStr := string(messageContent.MessageType)
		if messageContent.MessageType == models.Action {
			if payloadMap, ok := messageContent.Payload.(map[string]interface{}); ok {
				if at, ok := payloadMap["actionType"].(string); ok && at != "" {
					actionStr = at
				}
			}
		}

		allowed, err := g.validator.ValidateUserPermissions(cert, actionStr, g.locationPath, rootCA, intermediates)
		if err != nil {
			g.logger.Warnw("Permission validation error", "email", msg.Email, "action", actionStr, "location", g.locationPath, "error", err)
			return
		}
		if !allowed {
			g.logger.Warnw("Permission denied", "email", msg.Email, "action", actionStr, "location", g.locationPath)
			return
		}
	}

	select {
	case g.verifiedInbound <- &types.MessageWithSender{
		Content:     messageContent,
		SenderEmail: msg.Email,
		TraceID:     msg.TraceID,
	}:
	case <-ctx.Done():
		return
	}
}

func (g *Gatekeeper) processOutbound(ctx context.Context) {
	defer g.wg.Done()
	defer func() {
		r := recover()
		if r != nil {
			g.logger.Errorw("panic in processOutbound", "panic", r)
			sentry.ReportIssuefWithContext(sentry.IssueTypeError, g.logger, map[string]interface{}{"feature": string(deps.FeatureGatekeeper)}, "gatekeeper processOutbound panic: %v", r)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-g.verifiedOutbound:
			if !ok {
				return
			}
			g.handleOutbound(ctx, msg)
		}
	}
}

func (g *Gatekeeper) handleOutbound(ctx context.Context, msg *types.MessageWithSender) {
	encoded, err := g.compression.Encode(msg.Content)
	if err != nil {
		g.logger.Warnw("Failed to encode outbound message", "email", msg.SenderEmail, "error", err)
		return
	}

	encrypted := []byte(encoded)

	g.mu.RLock()
	instanceUUID := g.instanceUUID
	g.mu.RUnlock()

	transportMsg := &types.UMHMessage{
		InstanceUUID: instanceUUID,
		Content:      string(encrypted),
		Email:        msg.SenderEmail,
	}

	select {
	case g.outboundChan <- transportMsg:
	case <-ctx.Done():
		return
	}
}

// processLegacyOutbound converts pre-encoded models.UMHMessage to types.UMHMessage.
// TODO(ENG-4558): Remove once actions write MessageWithSender directly.
func (g *Gatekeeper) processLegacyOutbound(ctx context.Context) {
	defer g.wg.Done()
	defer func() {
		r := recover()
		if r != nil {
			g.logger.Errorw("panic in processLegacyOutbound", "panic", r)
			sentry.ReportIssuefWithContext(sentry.IssueTypeError, g.logger, map[string]interface{}{"feature": string(deps.FeatureGatekeeper)}, "gatekeeper processLegacyOutbound panic: %v", r)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-g.legacyOutbound:
			if !ok {
				return
			}
			g.mu.RLock()
			instanceUUID := g.instanceUUID
			g.mu.RUnlock()
			transportMsg := &types.UMHMessage{
				InstanceUUID: instanceUUID,
				Content:      msg.Content,
				Email:        msg.Email,
			}
			select {
			case g.outboundChan <- transportMsg:
			case <-ctx.Done():
				return
			}
		}
	}
}
