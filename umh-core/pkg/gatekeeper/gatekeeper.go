// Package gatekeeper provides security middleware between FSMv2 transport and
// the Router, handling protocol detection, encryption, compression, and permission validation.
package gatekeeper

import (
	"context"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/certificatehandler"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/compression"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/protocol"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/validator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// SubHandler provides access to the list of active subscribers.
// Implemented by subscriber.Handler.
type SubHandler interface {
	Subscribers() []string
}

// Gatekeeper sits between Transport and Business Logic layers.
// It receives raw transport.UMHMessage from PullWorker, processes them
// (decrypt, decompress, validate), and outputs MessageWithSender to the Router.
// The reverse path handles outbound messages from Router to PushWorker.
type Gatekeeper struct {
	// Raw channels (received, not owned)
	inboundChan  chan *transport.UMHMessage
	outboundChan chan *transport.UMHMessage

	// Verified channels (created and owned by gatekeeper)
	verifiedInbound  chan *models.MessageWithSender
	verifiedOutbound chan *models.MessageWithSender

	// Legacy outbound for pre-encoded models.UMHMessage from actions.
	// TODO(ENG-4558): Remove once actions write MessageWithSender directly.
	legacyOutbound chan *models.UMHMessage

	// Sub-components
	protocolDetector protocol.Detector
	compression      compression.Handler
	validator        validator.Validator
	certHandler      certificatehandler.Handler

	// External state (read-only)
	subHandler SubHandler

	// Config (set by options, or defaults)
	verifiedInboundSize  int
	verifiedOutboundSize int
	certFetchInterval    time.Duration

	// Runtime
	logger  *zap.SugaredLogger
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.RWMutex
	running bool
}

// New creates a new Gatekeeper.
func New(
	inbound chan *transport.UMHMessage,
	outbound chan *transport.UMHMessage,
	subHandler SubHandler,
	certHandler certificatehandler.Handler,
	v validator.Validator,
	logger *zap.SugaredLogger,
	opts ...Option,
) *Gatekeeper {
	g := &Gatekeeper{
		inboundChan:          inbound,
		outboundChan:         outbound,
		subHandler:            subHandler,
		certHandler:          certHandler,
		validator:            v,
		protocolDetector:     protocol.NewDetector(logger),
		compression:          compression.NewHandler(logger),
		verifiedInboundSize:  DefaultVerifiedInboundBufferSize,
		verifiedOutboundSize: DefaultVerifiedOutboundBufferSize,
		certFetchInterval:    DefaultCertFetchInterval,
		logger:               logger,
	}
	for _, opt := range opts {
		opt(g)
	}
	g.verifiedInbound = make(chan *models.MessageWithSender, g.verifiedInboundSize)
	g.verifiedOutbound = make(chan *models.MessageWithSender, g.verifiedOutboundSize)
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

	g.wg.Add(4)
	go g.processInbound(ctx)
	go g.processOutbound(ctx)
	go g.processLegacyOutbound(ctx)
	go g.fetchCertificates(ctx)
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
	close(g.verifiedInbound)
}

// VerifiedInboundChan returns the channel for the Router to read verified messages.
func (g *Gatekeeper) VerifiedInboundChan() <-chan *models.MessageWithSender {
	return g.verifiedInbound
}

// VerifiedOutboundChan returns the channel for the Router to write messages to send.
func (g *Gatekeeper) VerifiedOutboundChan() chan<- *models.MessageWithSender {
	return g.verifiedOutbound
}

// LegacyOutboundChan returns the channel for actions to write pre-encoded models.UMHMessage.
// TODO(ENG-4558): Remove once actions write MessageWithSender directly.
func (g *Gatekeeper) LegacyOutboundChan() chan *models.UMHMessage {
	return g.legacyOutbound
}

// GetChannels implements communicator.ChannelProvider.
func (g *Gatekeeper) GetChannels(_ string) (chan<- *transport.UMHMessage, <-chan *transport.UMHMessage) {
	return g.inboundChan, g.outboundChan
}

// GetInboundStats implements communicator.ChannelProvider.
func (g *Gatekeeper) GetInboundStats(_ string) (capacity int, length int) {
	return cap(g.inboundChan), len(g.inboundChan)
}

// CertificateHandler returns the certificate handler.
func (g *Gatekeeper) CertificateHandler() certificatehandler.Handler {
	return g.certHandler
}

func (g *Gatekeeper) processInbound(ctx context.Context) {
	defer g.wg.Done()

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

func (g *Gatekeeper) handleInbound(ctx context.Context, msg *transport.UMHMessage) {
	// 1. Detect protocol and get encryption handler
	cryptHandler := g.protocolDetector.Detect(msg)

	// 2. Decrypt
	decrypted, err := cryptHandler.Decrypt([]byte(msg.Content))
	if err != nil {
		g.logger.Warnw("Failed to decrypt message", "email", msg.Email, "error", err)
		return
	}

	// 3. Decompress and decode
	messageContent, err := g.compression.Decode(string(decrypted))
	if err != nil {
		g.logger.Warnw("Failed to decode message", "email", msg.Email, "error", err)
		return
	}

	// 4. Validate permissions
	cert := g.certHandler.Certificate(msg.Email)
	if cert != nil {
		rootCA := g.certHandler.RootCA()
		intermediates := g.certHandler.IntermediateCerts(msg.Email)
		allowed, err := g.validator.ValidateUserPermissions(cert, string(messageContent.MessageType), "", rootCA, intermediates)
		if err != nil {
			g.logger.Warnw("Permission validation error", "email", msg.Email, "error", err)
			return
		}
		if !allowed {
			g.logger.Warnw("Permission denied", "email", msg.Email, "messageType", messageContent.MessageType)
			return
		}
	}

	// 5. Send to verified channel
	select {
	case g.verifiedInbound <- &models.MessageWithSender{
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

func (g *Gatekeeper) handleOutbound(ctx context.Context, msg *models.MessageWithSender) {
	// 1. Encode and compress
	encoded, err := g.compression.Encode(msg.Content)
	if err != nil {
		g.logger.Warnw("Failed to encode outbound message", "email", msg.SenderEmail, "error", err)
		return
	}

	// 2. Encrypt (v0 = noop for now)
	encrypted := []byte(encoded)

	// 3. Wrap as transport.UMHMessage
	transportMsg := &transport.UMHMessage{
		Content: string(encrypted),
		Email:   msg.SenderEmail,
	}

	// 4. Send to outbound channel
	select {
	case g.outboundChan <- transportMsg:
	case <-ctx.Done():
		return
	}
}

// processLegacyOutbound converts pre-encoded models.UMHMessage to transport.UMHMessage.
// TODO(ENG-4558): Remove once actions write MessageWithSender directly.
func (g *Gatekeeper) processLegacyOutbound(ctx context.Context) {
	defer g.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-g.legacyOutbound:
			if !ok {
				return
			}
			transportMsg := &transport.UMHMessage{
				Content: msg.Content,
				Email:   msg.Email,
			}
			select {
			case g.outboundChan <- transportMsg:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (g *Gatekeeper) fetchCertificates(ctx context.Context) {
	defer g.wg.Done()

	ticker := time.NewTicker(g.certFetchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			emails := g.subHandler.Subscribers()
			sort.Strings(emails)
			for _, email := range emails {
				if ctx.Err() != nil {
					return
				}
				if err := g.certHandler.FetchAndStore(ctx, email); err != nil {
					g.logger.Warnw("Failed to fetch certificate", "email", email, "error", err)
				}
			}
		}
	}
}
