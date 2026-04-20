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

package certificatehandler

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"go.uber.org/zap"
	"golang.org/x/crypto/sha3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/validator"
)

// SubHandler provides access to the list of active subscribers.
type SubHandler interface {
	Subscribers() []string
}

// UserCertificateEndpoint is the Management Console API endpoint for user certificates.
const UserCertificateEndpoint = "/v2/instance/user/certificate"

// UserCertificateResponse represents the JSON response from the user certificate endpoint.
type UserCertificateResponse struct {
	UserEmail        string `json:"userEmail"`
	Certificate      string `json:"certificate"`
	CertificateChain string `json:"certificateChain"`
	RootCA           string `json:"encryptedRootCA"`
}

type certificateBundle struct {
	cert             *x509.Certificate
	certificateChain []*x509.Certificate
}

// CertHandler implements Handler with an in-memory cache and Management Console API fetching.
type CertHandler struct {
	validator  validator.Validator
	subHandler SubHandler
	rootCA     *x509.Certificate
	userCerts  map[string]*certificateBundle

	log             *zap.SugaredLogger
	encryptedRootCA string
	jwt             string
	apiURL          string
	authToken       string
	instanceUUID    string
	mu              sync.RWMutex

	insecureTLS bool
}

// NewHandler creates a CertHandler with the given API configuration.
func NewHandler(
	v validator.Validator,
	jwt string,
	apiURL string,
	authToken string,
	instanceUUID string,
	insecureTLS bool,
	log *zap.SugaredLogger,
) *CertHandler {
	return &CertHandler{
		userCerts:    make(map[string]*certificateBundle),
		validator:    v,
		jwt:          jwt,
		apiURL:       apiURL,
		authToken:    authToken,
		instanceUUID: instanceUUID,
		insecureTLS:  insecureTLS,
		log:          log,
	}
}

// SetJWT updates the JWT token used for API authentication.
func (h *CertHandler) SetJWT(jwt string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.jwt = jwt
}

// SetInstanceUUID updates the instance UUID used for root CA decryption.
func (h *CertHandler) SetInstanceUUID(uuid string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.instanceUUID = uuid
}

// Certificate returns the cached certificate for the given email, or nil if not found.
func (h *CertHandler) Certificate(email string) *x509.Certificate {
	h.mu.RLock()
	defer h.mu.RUnlock()

	cached := h.userCerts[strings.ToLower(email)]
	if cached != nil {
		return cached.cert
	}

	return nil
}

// IntermediateCerts returns the cached intermediate certificate chain for the given email.
func (h *CertHandler) IntermediateCerts(email string) []*x509.Certificate {
	h.mu.RLock()
	defer h.mu.RUnlock()

	cached := h.userCerts[strings.ToLower(email)]
	if cached != nil {
		return cached.certificateChain
	}

	return nil
}

// RootCA returns the cached root CA certificate, or nil if not yet fetched.
func (h *CertHandler) RootCA() *x509.Certificate {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.rootCA
}

// SetSubHandler sets the subscriber handler for resolving who to fetch certs for.
func (h *CertHandler) SetSubHandler(sh SubHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subHandler = sh
}

// HasSubHandler returns true if the subscriber handler is available.
func (h *CertHandler) HasSubHandler() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.subHandler != nil
}

// Subscribers returns the list of active subscriber emails.
func (h *CertHandler) Subscribers() []string {
	h.mu.RLock()
	sh := h.subHandler
	h.mu.RUnlock()
	if sh == nil {
		return nil
	}
	return sh.Subscribers()
}

// FetchAllCerts fetches certificates for all active subscribers.
func (h *CertHandler) FetchAllCerts(ctx context.Context) error {
	emails := h.Subscribers()
	if len(emails) == 0 {
		return nil
	}
	var lastErr error
	for _, email := range emails {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := h.fetchAndStore(ctx, email)
		if err != nil {
			h.log.Warnw("cert fetch failed", "email", email, "error", err)
			lastErr = err
		}
	}
	return lastErr
}

// fetchAndStore fetches the certificate for a single user from the Management Console API
// and stores it in the cache.
func (h *CertHandler) fetchAndStore(ctx context.Context, email string) error {
	h.mu.RLock()
	jwt := h.jwt
	h.mu.RUnlock()

	cookies := map[string]string{"token": jwt}

	normalizedEmail := strings.ToLower(email)

	resp, err := h.fetchUserCertificate(ctx, normalizedEmail, &cookies)
	if err != nil {
		return fmt.Errorf("fetch certificate for %s: %w", normalizedEmail, err)
	}

	if resp == nil {
		// HTTP 204: user has no certificate (revoked or not yet issued).
		// Clear any previously cached cert.
		h.deleteCertificate(normalizedEmail)
		return nil
	}

	cert, err := parseCertificate(resp.Certificate)
	if err != nil {
		return fmt.Errorf("parse certificate for %s: %w", email, err)
	}

	chain, err := parseCertificateChain(resp.CertificateChain)
	if err != nil {
		return fmt.Errorf("parse certificate chain for %s: %w", email, err)
	}

	h.setCertificate(strings.ToLower(resp.UserEmail), cert, chain)

	h.mu.RLock()
	encryptedRootCA := h.encryptedRootCA
	authToken := h.authToken
	instanceUUID := h.instanceUUID
	h.mu.RUnlock()

	if resp.RootCA != "" && resp.RootCA != encryptedRootCA {
		h.setEncryptedRootCA(resp.RootCA)

		hasher := sha3.New256()
		hasher.Write([]byte(authToken))
		keyMaterial := hex.EncodeToString(hasher.Sum(nil))

		decrypted, err := h.validator.DecryptRootCA(resp.RootCA, keyMaterial, instanceUUID)
		if err != nil {
			return fmt.Errorf("decrypt rootCA: %w", err)
		}

		rootCert, err := parseCertificate(decrypted)
		if err != nil {
			return fmt.Errorf("parse decrypted rootCA: %w", err)
		}

		h.setRootCA(rootCert)
	}

	return nil
}

func (h *CertHandler) setCertificate(email string, cert *x509.Certificate, intermediates []*x509.Certificate) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.userCerts[email] = &certificateBundle{
		cert:             cert,
		certificateChain: intermediates,
	}
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

func (h *CertHandler) fetchUserCertificate(ctx context.Context, email string, cookies *map[string]string) (*UserCertificateResponse, error) {
	encodedEmail := url.QueryEscape(email)
	endpoint := http.Endpoint(fmt.Sprintf("%s?email=%s", UserCertificateEndpoint, encodedEmail))

	h.mu.RLock()
	apiURL := h.apiURL
	insecureTLS := h.insecureTLS
	h.mu.RUnlock()

	response, statusCode, err := http.GetRequest[UserCertificateResponse](ctx, endpoint, nil, cookies, insecureTLS, apiURL, h.log)
	if err != nil {
		if statusCode == 204 {
			return nil, nil
		}

		return nil, err
	}

	return response, nil
}

func parseCertificate(base64Cert string) (*x509.Certificate, error) {
	derBytes, err := base64.StdEncoding.DecodeString(base64Cert)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert, nil
}

func parseCertificateChain(chainJSON string) ([]*x509.Certificate, error) {
	if chainJSON == "" || chainJSON == "[]" {
		return nil, nil
	}

	var base64Certs []string

	err := json.Unmarshal([]byte(chainJSON), &base64Certs)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal certificate chain: %w", err)
	}

	chain := make([]*x509.Certificate, 0, len(base64Certs))
	for _, base64Cert := range base64Certs {
		cert, err := parseCertificate(base64Cert)
		if err != nil {
			return nil, fmt.Errorf("failed to parse chain certificate: %w", err)
		}

		chain = append(chain, cert)
	}

	return chain, nil
}
