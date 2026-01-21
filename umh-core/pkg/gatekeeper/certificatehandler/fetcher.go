package certificatehandler

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http"
)

const UserCertificateEndpoint = "/v2/instance/user/certificate"

type UserCertificateResponse struct {
	UserEmail        string `json:"userEmail"`
	Certificate      string `json:"certificate"`
	CertificateChain string `json:"certificateChain"`
	RootCA           string `json:"encryptedRootCA"` // compare with stored encrypted
}

func (h *CertHandler) StartFetcher(ctx context.Context, jwt string, apiURL string, insecureTLS bool) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.refreshCertificates(ctx, jwt, apiURL, insecureTLS)
		}
	}
}

func (h *CertHandler) refreshCertificates(ctx context.Context, jwt string, apiURL string, insecureTLS bool) {
	h.mu.RLock()
	emails := make([]string, 0, len(h.userCerts))
	for email := range h.userCerts {
		emails = append(emails, email)
	}
	h.mu.RUnlock()

	cookies := map[string]string{"token": jwt}

	for _, email := range emails {
		resp, err := h.fetchUserCertificate(ctx, email, &cookies, insecureTLS, apiURL)
		if err != nil {
			h.log.Warnf("Failed to fetch certificate for %s: %v", email, err)
			continue
		}

		if resp == nil {
			continue
		}

		cert, err := parseCertificate(resp.Certificate)
		if err != nil {
			h.log.Warnf("Failed to parse certificate for %s: %v", email, err)
			continue
		}

		chain, err := parseCertificateChain(resp.CertificateChain)
		if err != nil {
			h.log.Warnf("Failed to parse certificate chain for %s: %v", email, err)
			continue
		}

		h.setCertificate(resp.UserEmail, cert, chain)

		if resp.RootCA != "" && resp.RootCA != h.encryptedRootCA {
			h.setEncryptedRootCA(resp.RootCA)
			// TODO: decrypt and parse rootCA
		}
	}
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

func (h *CertHandler) fetchUserCertificate(ctx context.Context, email string, cookies *map[string]string, insecureTLS bool, apiURL string) (*UserCertificateResponse, error) {
	encodedEmail := url.QueryEscape(email)
	endpoint := http.Endpoint(fmt.Sprintf("%s?email=%s", UserCertificateEndpoint, encodedEmail))

	response, statusCode, err := http.GetRequest[UserCertificateResponse](ctx, endpoint, nil, cookies, insecureTLS, apiURL, h.log)
	if err != nil {
		if statusCode == 204 {
			return nil, nil
		}

		return nil, err
	}

	return response, nil
}
