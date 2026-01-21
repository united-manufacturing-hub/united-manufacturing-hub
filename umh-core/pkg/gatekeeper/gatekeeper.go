package gatekeeper

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/certificatehandler"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/compression"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/protocol"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/validator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// Gatekeeper sits between Transport and Business Logic layers.
type Gatekeeper struct {
	validator          validator.Validator
	certificateHandler certificatehandler.Handler
	protocolDetector   protocol.Detector
	compression        compression.Handler

	inboundChan  <-chan *models.UMHMessage
	outboundChan chan<- *models.UMHMessage
	verifiedChan chan *models.MessageContentWithSender

	log     *zap.SugaredLogger
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.RWMutex
}

// New creates a new Gatekeeper.
func New(
	inbound <-chan *models.UMHMessage,
	outbound chan<- *models.UMHMessage,
	certHandler certificatehandler.Handler,
	log *zap.SugaredLogger,
) *Gatekeeper {
	return &Gatekeeper{
		validator:          validator.NewValidator(log),
		certificateHandler: certHandler,
		protocolDetector:   protocol.NewDetector(log),
		compression:        compression.NewHandler(log),
		inboundChan:        inbound,
		outboundChan:       outbound,
		verifiedChan:       make(chan *models.MessageContentWithSender, 100),
		log:                log,
	}
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

	g.wg.Add(1)
	go g.processInbound(ctx)
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
	close(g.verifiedChan)
}

// VerifiedChannel returns the channel for verified messages.
func (g *Gatekeeper) VerifiedChannel() <-chan *models.MessageContentWithSender {
	return g.verifiedChan
}

// CertificateHandler returns the certificate handler.
func (g *Gatekeeper) CertificateHandler() certificatehandler.Handler {
	return g.certificateHandler
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
			g.handleMessage(ctx, msg)
		}
	}
}

func (g *Gatekeeper) handleMessage(_ context.Context, msg *models.UMHMessage) {
	// 1. Get the appropriate crypto handler based on protocol version
	cryptHandler := g.protocolDetector.Detect(msg)

	// 2. Decrypt content (no-op for v0, actual decryption for cseV1)
	decrypted, err := cryptHandler.Decrypt([]byte(msg.Content))
	if err != nil {
		g.log.Warnw("Failed to decrypt message", "email", msg.Email, "error", err)
		return
	}

	// 3. Decode (base64 + decompress + unmarshal)
	messageContent, err := g.compression.Decode(string(decrypted))
	if err != nil {
		g.log.Warnw("Failed to decode message", "email", msg.Email, "error", err)
		return
	}

	// TODO: 4. Validate permissions
	// TODO: 5. Send to verified channel

	g.log.Debugw("Processing message", "email", msg.Email, "messageType", messageContent.MessageType)
}
