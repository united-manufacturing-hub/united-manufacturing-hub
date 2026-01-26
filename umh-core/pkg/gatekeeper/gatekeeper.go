package gatekeeper

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/certificatehandler"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/compression"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/protocol"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/validator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

const subscriberTTL = 5 * time.Minute

type userFetcherState struct {
	cancel   context.CancelFunc
	lastSeen time.Time
}

// Gatekeeper sits between Transport and Business Logic layers.
type Gatekeeper struct {
	validator          validator.Validator
	certificateHandler certificatehandler.Handler
	protocolDetector   protocol.Detector
	compression        compression.Handler

	fetcher        certificatehandler.Fetcher
	activeFetchers sync.Map

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
	fetcher certificatehandler.Fetcher,
	log *zap.SugaredLogger,
) *Gatekeeper {
	return &Gatekeeper{
		validator:          validator.NewValidator(log),
		certificateHandler: certHandler,
		protocolDetector:   protocol.NewDetector(log),
		compression:        compression.NewHandler(log),
		fetcher:            fetcher,
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

	g.wg.Add(2)
	go g.processInbound(ctx)
	go g.runFetcherCleanup(ctx)
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

	g.activeFetchers.Range(func(key, value any) bool {
		state := value.(*userFetcherState)
		state.cancel()
		return true
	})

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

func (g *Gatekeeper) handleMessage(ctx context.Context, msg *models.UMHMessage) {
	cryptHandler := g.protocolDetector.Detect(msg)

	decrypted, err := cryptHandler.Decrypt([]byte(msg.Content))
	if err != nil {
		g.log.Warnf("Failed to decrypt message: %v", err)
		return
	}

	messageContent, err := g.compression.Decode(string(decrypted))
	if err != nil {
		g.log.Warnf("Failed to decode message: %v", err)
		return
	}

	if messageContent.MessageType == models.Subscribe {
		g.manageFetcher(ctx, msg.Email, true)
	}

	// TODO: Validate permissions
	// TODO: Send to verified channel

	g.log.Debugf("Processing message type=%s email=%s", messageContent.MessageType, msg.Email)
}

func (g *Gatekeeper) manageFetcher(ctx context.Context, email string, active bool) {
	if active {
		existing, loaded := g.activeFetchers.Load(email)
		if loaded {
			state := existing.(*userFetcherState)
			state.lastSeen = time.Now()
			return
		}
		g.addFetcher(ctx, email)
	} else {
		g.removeFetcher(email)
	}
}

func (g *Gatekeeper) addFetcher(ctx context.Context, email string) {
	fetcherCtx, cancel := context.WithCancel(ctx)
	state := &userFetcherState{
		cancel:   cancel,
		lastSeen: time.Now(),
	}

	if _, loaded := g.activeFetchers.LoadOrStore(email, state); loaded {
		cancel()
		return
	}

	g.log.Debugf("Starting certificate fetcher for %s", email)
	go g.fetcher.RunForUser(fetcherCtx, email)
}

func (g *Gatekeeper) removeFetcher(email string) {
	existing, loaded := g.activeFetchers.LoadAndDelete(email)
	if !loaded {
		return
	}

	state := existing.(*userFetcherState)
	state.cancel()
	g.log.Debugf("Stopped certificate fetcher for %s", email)
}

func (g *Gatekeeper) runFetcherCleanup(ctx context.Context) {
	defer g.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			g.activeFetchers.Range(func(key, value any) bool {
				email := key.(string)
				state := value.(*userFetcherState)

				if now.Sub(state.lastSeen) > subscriberTTL {
					g.manageFetcher(ctx, email, false)
				}

				return true
			})
		}
	}
}
