package certificatehandler

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/sha3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/validator"
)

const UserCertificateEndpoint = "/v2/instance/user/certificate"

type UserCertificateResponse struct {
	UserEmail        string `json:"userEmail"`
	Certificate      string `json:"certificate"`
	CertificateChain string `json:"certificateChain"`
	RootCA           string `json:"encryptedRootCA"`
}

type CertFetcher struct {
	handler      *CertHandler
	validator    validator.Validator
	jwt          string
	apiURL       string
	authToken    string
	instanceUUID string
	insecureTLS  bool
	log          *zap.SugaredLogger
}

func NewFetcher(
	handler *CertHandler,
	v validator.Validator,
	jwt string,
	apiURL string,
	authToken string,
	instanceUUID string,
	insecureTLS bool,
	log *zap.SugaredLogger,
) *CertFetcher {
	return &CertFetcher{
		handler:      handler,
		validator:    v,
		jwt:          jwt,
		apiURL:       apiURL,
		authToken:    authToken,
		instanceUUID: instanceUUID,
		insecureTLS:  insecureTLS,
		log:          log,
	}
}

func (f *CertFetcher) RunForUser(ctx context.Context, email string) {
	f.fetchAndStore(ctx, email)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			f.handler.deleteCertificate(email)
			return
		case <-ticker.C:
			f.fetchAndStore(ctx, email)
		}
	}
}

func (f *CertFetcher) fetchAndStore(ctx context.Context, email string) {
	cookies := map[string]string{"token": f.jwt}

	resp, err := f.fetchUserCertificate(ctx, email, &cookies)
	if err != nil {
		f.log.Warnf("Failed to fetch certificate for %s: %v", email, err)
		return
	}

	if resp == nil {
		return
	}

	cert, err := parseCertificate(resp.Certificate)
	if err != nil {
		f.log.Warnf("Failed to parse certificate for %s: %v", email, err)
		return
	}

	chain, err := parseCertificateChain(resp.CertificateChain)
	if err != nil {
		f.log.Warnf("Failed to parse certificate chain for %s: %v", email, err)
		return
	}

	f.handler.setCertificate(resp.UserEmail, cert, chain)

	if resp.RootCA != "" && resp.RootCA != f.handler.encryptedRootCA {
		f.handler.setEncryptedRootCA(resp.RootCA)

		hasher := sha3.New256()
		hasher.Write([]byte(f.authToken))
		keyMaterial := hex.EncodeToString(hasher.Sum(nil))

		decrypted, err := f.validator.DecryptRootCA(resp.RootCA, keyMaterial, f.instanceUUID)
		if err != nil {
			f.log.Warnf("Failed to decrypt rootCA: %v", err)
			return
		}

		rootCert, err := parseCertificate(decrypted)
		if err != nil {
			f.log.Warnf("Failed to parse decrypted rootCA: %v", err)
			return
		}

		f.handler.setRootCA(rootCert)
	}
}

func (f *CertFetcher) fetchUserCertificate(ctx context.Context, email string, cookies *map[string]string) (*UserCertificateResponse, error) {
	encodedEmail := url.QueryEscape(email)
	endpoint := http.Endpoint(fmt.Sprintf("%s?email=%s", UserCertificateEndpoint, encodedEmail))

	response, statusCode, err := http.GetRequest[UserCertificateResponse](ctx, endpoint, nil, cookies, f.insecureTLS, f.apiURL, f.log)
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
