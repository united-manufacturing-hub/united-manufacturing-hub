package certificatehandler

import (
	"crypto/x509"
	"maps"
	"sync"

	"go.uber.org/zap"
)

type certificateBundle struct {
	cert             *x509.Certificate
	certificateChain []*x509.Certificate
}

type CertHandler struct {
	rootCA          *x509.Certificate
	encryptedRootCA string
	userCerts       map[string]*certificateBundle
	mu              sync.RWMutex
	log             *zap.SugaredLogger
}

func NewHandler(log *zap.SugaredLogger) *CertHandler {
	return &CertHandler{
		userCerts: make(map[string]*certificateBundle),
		log:       log,
	}
}

func (h *CertHandler) GetCertificate(email string) *x509.Certificate {
	h.mu.RLock()
	defer h.mu.RUnlock()

	cached := h.userCerts[email]
	if cached != nil {
		return cached.cert
	}

	return nil
}

func (h *CertHandler) GetIntermediateCerts(email string) []*x509.Certificate {
	h.mu.RLock()
	defer h.mu.RUnlock()

	cached := h.userCerts[email]
	if cached != nil {
		return cached.certificateChain
	}

	return nil
}

func (h *CertHandler) GetRootCA() *x509.Certificate {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.rootCA
}

func (h *CertHandler) setCertificate(email string, cert *x509.Certificate, intermediates []*x509.Certificate) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.userCerts[email] = &certificateBundle{
		cert:             cert,
		certificateChain: intermediates,
	}
}

func (h *CertHandler) updateCerts(certs map[string]*certificateBundle) {
	h.mu.Lock()
	defer h.mu.Unlock()

	maps.Copy(h.userCerts, certs)
}

func (h *CertHandler) deleteCertificate(email string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.userCerts, email)
}

func (h *CertHandler) setRootCA(rootCA *x509.Certificate) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.rootCA = rootCA
}

func (h *CertHandler) setEncryptedRootCA(encrypted string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.encryptedRootCA = encrypted
}

func (h *CertHandler) deleteRootCA() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.rootCA = nil
}

func (h *CertHandler) trackEmail(email string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.userCerts[email]; !exists {
		h.userCerts[email] = nil
	}
}
