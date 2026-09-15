package v2

import (
	"crypto/rand"
	"crypto/sha3"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net"

	"github.com/united-manufacturing-hub/ManagementConsole/cryptolib/certificate"
	"github.com/united-manufacturing-hub/ManagementConsole/cryptolib/encoder"
	"github.com/united-manufacturing-hub/ManagementConsole/cryptolib/helper"
	"github.com/united-manufacturing-hub/ManagementConsole/cryptolib/password_derivation"
	"github.com/united-manufacturing-hub/ManagementConsole/cryptolib/permission_validator"
)

// AuthorizationCrypto defines the interface for Layer 2 authorization certificate operations exposed to the frontend and UMH Core.
//
// This interface serves as the public API for all Layer 2 (Authorization) cryptographic functions in the UMH system.
// Layer 2 handles certificate-based authorization, permission validation, and user/instance certificate management.
// All functions in this interface are exposed to both the web frontend (via WASM) and UMH Core instances.
//
// Layer 2 responsibilities include:
// - Certificate generation (CA, user, instance certificates)
// - Permission validation for actions at specific locations
// - Role-based access control (Admin, Editor, Viewer roles)
// - Certificate lifecycle management (password changes, version detection)
// - Location-based permission enforcement
//
// To expose a new certificate function to the frontend, follow these steps:
//
//  1. Add the function signature to this interface
//     Example: GetUserRoles(certBundle CertificateBundle) (certificate.LocationRoles, error)
//
//  2. Implement the function in the GoCrypto struct (below in this file)
//     The implementation should handle all business logic and validation
//
//  3. Create a WASM wrapper in cryptomodule/cmd/main.go:
//     a) Create two functions - a public wrapper and an internal implementation:
//     func getUserRoles(_ js.Value, args []js.Value) interface{} {
//     return wrapResult(getUserRolesInternal(args))
//     }
//     func getUserRolesInternal(args []js.Value) interface{} { ... }
//     b) Parse JS arguments and convert to Go types
//     c) Call the GoCrypto method from step 2
//     d) Convert the result to JSON-serializable format
//     e) Return the result (wrapResult handles success/error wrapping)
//
//  4. Register the callback at the bottom of cryptomodule/cmd/main.go:
//     In registerCallbacks(), add:
//     js.Global().Set("getUserRoles", js.FuncOf(getUserRoles))
//
//  5. Add the function signature to frontend/src/lib/utils/crypto/crypto.ts:
//     a) In the Window interface declaration, add:
//     getUserRoles: (certificate: string) => string;
//     b) Create a public method in the CryptoModule class:
//     public async getUserRoles(certificate: string): Promise<Record<string, string>> {
//     await this.ensureLoaded();
//     return this.parseWASMResult<string>(window.getUserRoles(certificate));
//     }
//
// The WASM module is compiled from the Go code and loaded by the frontend at runtime.
// All data exchange happens through JSON serialization, so complex types must be
// converted to/from JSON-compatible formats.
type AuthorizationCrypto interface {
	ValidateUserPermissions(userCert *x509.Certificate, actionStr string, locationStr string, rootCA *x509.Certificate, intermediateCerts []*x509.Certificate) (bool, error)
	GenerateCAAndOwnerCertificate(caRequest CACertRequest, ownerPassword string) (CertificateBundle, CertificateBundle, error)
	GenerateUserCertificate(certRequest UserCertRequest, adminCertBundle CertificateBundle, adminPassword string) (CertificateBundle, string, error)
	ChangePassword(certBundle CertificateBundle, oldPassword, newPassword string, email string) (CertificateBundle, error)
	GenerateInstanceCertificate(certRequest InstanceCertRequest, adminCertBundle CertificateBundle, adminPassword string) (CertificateBundle, string, error)
	GenerateInstanceCertificateV2(certRequest InstanceCertRequest, adminCertBundle CertificateBundle, adminPassword string) (CertificateBundle, string, error)
	GetCertificateVersion(certBundle CertificateBundle) (string, error)
	GetUserRoles(certBundle CertificateBundle) (certificate.LocationRoles, error)
	IsInviteAllowedForLocation(inviterCert *x509.Certificate, locationStr string) error
	EncryptRootCA(caCertPEM string, keyMaterial string, salt string) (string, error)
	DecryptRootCA(encryptedCA string, keyMaterial string, salt string) (string, error)
}

type CertificateBundle struct {
	cert             *x509.Certificate
	encryptedKey     string
	certificateChain []string // Base64-encoded intermediate certificates
}

// NewCertificateBundle creates a new CertificateBundle
func NewCertificateBundle(cert *x509.Certificate, encryptedKey string) CertificateBundle {
	return CertificateBundle{
		cert:             cert,
		encryptedKey:     encryptedKey,
		certificateChain: []string{},
	}
}

// NewCertificateBundleWithChain creates a new CertificateBundle with a certificate chain
func NewCertificateBundleWithChain(cert *x509.Certificate, encryptedKey string, chain []string) CertificateBundle {
	return CertificateBundle{
		cert:             cert,
		encryptedKey:     encryptedKey,
		certificateChain: chain,
	}
}

// GetCertificate returns the certificate from the bundle
func (cb CertificateBundle) GetCertificate() *x509.Certificate {
	return cb.cert
}

// GetEncryptedKey returns the encrypted private key from the bundle
func (cb CertificateBundle) GetEncryptedKey() string {
	return cb.encryptedKey
}

// GetCertificateChain returns the certificate chain from the bundle
func (cb CertificateBundle) GetCertificateChain() []string {
	return cb.certificateChain
}

// UserCertRequest represents a request to generate a user certificate with location-based roles.
type UserCertRequest struct {
	LocationRoles certificate.LocationRoles
	Organization  string
	Mail          string
	AdminEmail    string // Email of the admin signing the certificate
}

// CACertRequest represents a request to generate a Certificate Authority certificate.
type CACertRequest struct {
	Organization string
	OwnerMail    string
}

// InstanceCertRequest represents a request to generate an instance certificate for automated systems.
type InstanceCertRequest struct {
	Organization      string
	AdminEmail        string // Email of the admin signing the certificate
	InstanceAuthToken string // Auth token used to derive instance certificate encryption password
	InstanceUUID      string // UUID for the instance (used as CommonName)
}

// GoCrypto implements the AuthorizationCrypto interface for V2 certificate operations
type GoCrypto struct {
}

// NewGoCrypto creates a new instance of GoCrypto
func NewGoCrypto() *GoCrypto {
	return &GoCrypto{}
}

// Ensure GoCrypto implements the AuthorizationCrypto interface
var _ AuthorizationCrypto = (*GoCrypto)(nil)

// ActionValidateChainOnly is a special action string that causes ValidateUserPermissions
// to only validate the certificate chain without checking any specific action permissions.
// This is useful when you only need to verify that a user's certificate is valid and
// properly signed by the CA chain, without checking authorization for a specific action.
const ActionValidateChainOnly = "validate-chain-only"

// ValidateUserPermissions validates whether a user has permission to execute a
// specific action at a given location within the UMH system.
//
// This function is called in the umh-core instance whenever a new action request
// comes in. It performs several validation steps:
// 1. Validates the user certificate against the certificate chain (rootCA + intermediates)
// 2. Extracts the user's role for the specified location from the certificate
// 3. Checks if that role is allowed to perform the requested action
//
// For v1 certificates, the function converts the location string to hierarchies
// to check against certificate hierarchies. For v2 certificates, it uses the
// location-based roles directly.
//
// Special case: If actionStr is "validate-chain-only" (ActionValidateChainOnly constant),
// the function only validates the certificate chain and returns true without checking
// any specific action permissions. This is useful for login validation where you only
// need to verify the certificate is valid.
//
// Parameters:
//   - userCert: the X.509 certificate of the user attempting the action. If nil,
//     the function returns false immediately.
//   - actionStr: the action being requested (e.g., "get-protocol-converter",
//     "set-configuration"). This is matched against permission rules.
//     Use "validate-chain-only" to skip permission checks and only validate the chain.
//   - locationStr: the location where the action is being performed, typically
//     in hierarchical format (e.g., "enterprise.site.area").
//     Ignored when actionStr is "validate-chain-only".
//   - rootCA: the root Certificate Authority certificate used to validate the
//     certificate chain.
//   - intermediateCerts: slice of intermediate certificates in the chain between
//     the root CA and the user certificate.
//
// Returns:
//   - bool: true if the user has permission to perform the action at the location,
//     false otherwise.
//   - error: non-nil if certificate validation fails, role extraction fails, or
//     other validation errors occur. Returns nil error when permission is denied
//     due to insufficient privileges.
func (g *GoCrypto) ValidateUserPermissions(userCert *x509.Certificate, actionStr string,
	locationStr string, rootCA *x509.Certificate, intermediateCerts []*x509.Certificate) (bool, error) {
	if userCert == nil {
		return false, nil
	}

	err := certificate.ValidateX509UserCertificate(userCert, rootCA, intermediateCerts)
	if err != nil {
		return false, err
	}

	// Special case: if action is "validate-chain-only", we only validate the chain
	// and return true without checking any specific permissions
	if actionStr == ActionValidateChainOnly {
		return true, nil
	}

	// Try V2 first: attempt to get role via LocationRoles extension
	role, err := certificate.GetRoleForLocation(userCert, locationStr)
	if err != nil {
		// Check if this is specifically a missing V2 extension error
		if errors.Is(err, certificate.ERR_RoleLocation_Not_Found) {
			// V1 fallback: use V1 role and hierarchy validation
			v1Role, v1Err := certificate.GetRoleFromCertificate(userCert)
			if v1Err != nil {
				return false, fmt.Errorf("failed to get V1 role from certificate: %w", v1Err)
			}

			// Check if the location is allowed by the V1 hierarchies
			allowed, hierarchyErr := certificate.IsLocationAllowedByHierarchies(userCert, locationStr)
			if hierarchyErr != nil {
				return false, fmt.Errorf("failed to validate V1 hierarchies: %w", hierarchyErr)
			}

			if !allowed {
				// Location not permitted by V1 hierarchies
				return false, nil
			}

			// Location is permitted, now check if role allows the action
			actionAllowed := permission_validator.IsAllowedForAction(v1Role, actionStr)
			return actionAllowed, nil
		}

		// Some other error occurred during V2 processing
		return false, err
	}

	// V2 path succeeded, check if role allows the action
	allowed := permission_validator.IsAllowedForAction(role, actionStr)

	return allowed, nil
}

// GenerateCAAndOwnerCertificate creates a Certificate Authority (CA) certificate
// and an owner certificate when a new user registers in the Management Console.
//
// This function generates two certificates:
// 1. A root CA certificate that is self-signed and will be stored in the enterprise database
// 2. An owner certificate that is a user certificate with admin rights for all locations ("*")
//
// Both private keys are encrypted using passwords derived from the owner's email and password.
// The CA uses CA-specific password derivation, while the owner certificate uses user-specific
// password derivation.
//
// Parameters:
//   - caRequest: contains the organization name (Organization) and owner's email
//     (OwnerMail) for the new CA and owner certificate.
//   - ownerPassword: the owner's chosen password used to derive encryption keys
//     for both the CA and owner private keys.
//
// Returns:
//   - CertificateBundle (first): the CA certificate and its encrypted private key.
//     This should be stored in the enterprise database.
//   - CertificateBundle (second): the owner certificate and its encrypted private key.
//     The owner has admin rights for all locations ("*").
//   - error: non-nil if any step (CA generation, owner certificate generation,
//     key encryption) fails.
func (g *GoCrypto) GenerateCAAndOwnerCertificate(caRequest CACertRequest, ownerPassword string) (CertificateBundle, CertificateBundle, error) {
	// Generate the V2 CA certificate and private key
	caCert, caPrivateKey, err := certificate.GenerateX509CACertificateAndKeyV2(certificate.CARequest{
		Organization: caRequest.Organization,
	})
	if err != nil {
		return CertificateBundle{}, CertificateBundle{}, errors.Join(err, errors.New("failed to generate CA certificate"))
	}

	// Derive password for CA key encryption using email and password
	caKeyPassword := password_derivation.DeriveCAEncryptionKeyFromPassword(ownerPassword, caRequest.OwnerMail)

	// Encrypt and encode the CA private key
	caEncryptedKey, err := encoder.EncodeEd25519PrivateKey(caPrivateKey, caKeyPassword)
	if err != nil {
		return CertificateBundle{}, CertificateBundle{}, errors.Join(err, errors.New("failed to encrypt CA private key"))
	}

	// Create CA certificate bundle
	caBundle := NewCertificateBundle(caCert, caEncryptedKey)

	// Create location roles for the owner - admin rights for all locations
	ownerLocationRoles := certificate.LocationRoles{
		"*": certificate.RoleAdmin,
	}

	// Generate the owner certificate (user certificate with admin rights)
	ownerCert, ownerPrivateKey, err := certificate.GenerateX509UserCertificateAndKeyV2(
		certificate.CertificateRequestV2{
			Organization:  caRequest.Organization,
			CommonName:    caRequest.OwnerMail, // Use email as common name for owner
			Role:          certificate.RoleAdmin,
			LocationRoles: ownerLocationRoles,
			Hierarchies:   []certificate.LocationHierarchy{}, // Will get default hierarchy in the certificate generation
		},
		caCert,
		caPrivateKey,
	)
	if err != nil {
		return CertificateBundle{}, CertificateBundle{}, errors.Join(err, errors.New("failed to generate owner certificate"))
	}

	// Derive password for owner key encryption
	ownerKeyPassword := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(ownerPassword, caRequest.OwnerMail)

	// Encrypt and encode the owner private key
	ownerEncryptedKey, err := encoder.EncodeEd25519PrivateKey(ownerPrivateKey, ownerKeyPassword)
	if err != nil {
		return CertificateBundle{}, CertificateBundle{}, errors.Join(err, errors.New("failed to encrypt owner private key"))
	}

	// Create owner certificate bundle
	ownerBundle := NewCertificateBundle(ownerCert, ownerEncryptedKey)

	return caBundle, ownerBundle, nil
}

// GenerateUserCertificate creates an X.509 user certificate and encrypted private
// key for a new user invitation. This is called when an admin creates a new invite.
//
// The admin identified by certRequest.AdminEmail must sign the user certificate.
// The admin's private key is retrieved by decrypting adminCertBundle.GetEncryptedKey()
// using a password derived from adminPassword and certRequest.AdminEmail. Only admin
// user certificates are supported as signers - root CA certificates are not allowed
// to sign user certificates directly.
//
// The user certificate is generated with location-based roles specified in
// certRequest.LocationRoles, which define what permissions the user has at
// different locations. A random invite key is generated as the initial password
// for the user and must be changed on first login. The invite key is displayed
// to the admin and should be passed to the invited user.
//
// Design decision: user certificates can only be signed with admin user certificates
// we do not allow to sign user certificates with CA enterprise certificates
//
// Parameters:
//   - certRequest: contains the user's email (Mail), location-based roles
//     (LocationRoles), organization name (Organization), and the admin's email
//     (AdminEmail) who is signing the certificate.
//   - adminCertBundle: certificate and encrypted private key for the admin or CA
//     who will sign the user certificate.
//   - adminPassword: passphrase used (with AdminEmail) to derive the admin key
//     decryption password.
//
// Returns:
//   - CertificateBundle: the user certificate and encrypted private key.
//   - string: the randomly generated invite key that serves as the initial
//     password for the user. This must be changed on first login.
//   - error: non-nil if any step (admin key decode, certificate generation,
//     private key encryption) fails.
func (g *GoCrypto) GenerateUserCertificate(certRequest UserCertRequest, adminCertBundle CertificateBundle, adminPassword string) (CertificateBundle, string, error) {
	adminCert := adminCertBundle.GetCertificate()
	adminEncryptedKey := adminCertBundle.GetEncryptedKey()

	if adminCert == nil {
		return CertificateBundle{}, "", errors.New("admin certificate is nil")
	}

	// Early validation for all required fields
	if certRequest.AdminEmail == "" {
		return CertificateBundle{}, "", errors.New("admin email is required")
	}
	if certRequest.Organization == "" {
		return CertificateBundle{}, "", errors.New("organization is required")
	}
	if certRequest.Mail == "" {
		return CertificateBundle{}, "", errors.New("user email is required")
	}
	if len(certRequest.LocationRoles) == 0 {
		return CertificateBundle{}, "", errors.New("roles are required")
	}

	// Use the provided admin email for password derivation
	adminEmail := certRequest.AdminEmail

	// Validate signer identity and privileges
	if adminCert.Subject.CommonName != adminEmail {
		return CertificateBundle{}, "", errors.New("admin email does not match admin certificate CommonName")
	}
	if role, err := certificate.GetRoleFromCertificate(adminCert); err != nil || role != certificate.RoleAdmin {
		return CertificateBundle{}, "", errors.New("admin certificate must have Admin role")
	}
	if !adminCert.IsCA || (adminCert.KeyUsage&x509.KeyUsageCertSign == 0) {
		return CertificateBundle{}, "", errors.New("admin certificate is not CA-capable (IsCA/KeyUsageCertSign required)")
	}

	// Derive the admin key password - only admin user certificates are allowed to sign user certificates
	adminKeyPassword := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(adminPassword, adminEmail)

	// Decode the admin private key
	adminPrivateKey, err := encoder.DecodeEd25519PrivateKey(adminEncryptedKey, adminKeyPassword)
	if err != nil {
		return CertificateBundle{}, "", errors.Join(err, errors.New("failed to decode admin private key"))
	}

	// Generate a random invite key (4096 bytes)
	inviteKeyMaterial := make([]byte, 4096)
	_, err = rand.Read(inviteKeyMaterial)
	if err != nil {
		return CertificateBundle{}, "", errors.Join(err, errors.New("failed to generate random invite key"))
	}

	// Hash it with SHA-3-256 for sufficient security
	inviteKey := sha3.Sum256(inviteKeyMaterial)
	inviteKeyStr := hex.EncodeToString(inviteKey[:])

	// Generate legacy hierarchies from location roles for backwards compatibility
	hierarchies, roleV2 := helper.GenerateLegacyExtensionsFromLocationRoles(certRequest.LocationRoles)

	// Generate user certificate using V2 method
	userCert, userPrivateKey, err := certificate.GenerateX509UserCertificateAndKeyV2(
		certificate.CertificateRequestV2{
			Organization:  certRequest.Organization,
			CommonName:    certRequest.Mail,
			Role:          roleV2,
			Hierarchies:   hierarchies,
			LocationRoles: certRequest.LocationRoles,
		},
		adminCert,
		adminPrivateKey,
	)
	if err != nil {
		return CertificateBundle{}, "", errors.Join(err, errors.New("failed to generate user certificate"))
	}

	// Derive password for user key encryption using the invite key
	userKeyPassword := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(inviteKeyStr, certRequest.Mail)

	// Encrypt and encode the user private key
	userEncryptedKey, err := encoder.EncodeEd25519PrivateKey(userPrivateKey, userKeyPassword)
	if err != nil {
		return CertificateBundle{}, "", errors.Join(err, errors.New("failed to encrypt user private key"))
	}

	// Construct the certificate chain
	// Admin's chain + admin's own certificate = full chain for this user
	adminChain := adminCertBundle.GetCertificateChain()
	userChain := append(adminChain, encoder.EncodeX509Certificate(adminCert))

	// Create user certificate bundle with chain
	userBundle := NewCertificateBundleWithChain(userCert, userEncryptedKey, userChain)

	return userBundle, inviteKeyStr, nil
}

// ChangePassword re-encrypts the private key in a certificate bundle with a new password.
//
// This function decrypts the private key using the old password and re-encrypts it
// with the new password. The certificate itself remains unchanged, only the private
// key encryption is updated.
//
// For user certificates, the email is extracted from the certificate's CommonName
// field and used with the password to derive the encryption key. The function
// does not support CA certificates without a CommonName (root CA certificates)
// as they lack the email required for password derivation.
//
// Parameters:
//   - certBundle: the certificate bundle containing the certificate and encrypted
//     private key to be re-encrypted.
//   - oldPassword: the current password used to decrypt the private key. This is
//     combined with the email from the certificate's CommonName to derive the
//     current decryption key.
//   - newPassword: the new password to use for encrypting the private key. This is
//     combined with the email from the certificate's CommonName to derive the
//     new encryption key.
//   - email: email used to derive the passwords
//
// Returns:
//   - CertificateBundle: a new bundle with the same certificate but private key
//     encrypted with the new password.
//   - error: non-nil if the certificate is nil, CommonName is empty (CA certificates
//     without email), old password is incorrect, or re-encryption fails.
//
// Limitations:
//   - Does not support root CA certificates that have empty CommonName fields.
func (g *GoCrypto) ChangePassword(certBundle CertificateBundle, oldPassword, newPassword string, email string) (CertificateBundle, error) {
	cert := certBundle.GetCertificate()
	encryptedKey := certBundle.GetEncryptedKey()

	if cert == nil {
		return CertificateBundle{}, errors.New("certificate is nil")
	}

	// For CA certificates, we can't extract email from CommonName as it's not set
	// However, admin user certificates also have IsCA=true but do have CommonName
	if cert.IsCA && len(cert.Subject.CommonName) == 0 {
		return CertificateBundle{}, errors.New("enterprise CA certificate password change not supported")
	}

	// For user certificates, use user password derivation
	oldDerivedPassword := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(oldPassword, email)
	newDerivedPassword := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(newPassword, email)

	// Re-encode the private key with the new password
	newEncryptedKey, err := encoder.ReEncodeEd25519PrivateKey(encryptedKey, oldDerivedPassword, newDerivedPassword)
	if err != nil {
		return CertificateBundle{}, errors.Join(err, errors.New("failed to re-encode private key with new password"))
	}

	// Create new certificate bundle with the new encrypted key, preserving the chain
	newBundle := NewCertificateBundleWithChain(cert, newEncryptedKey, certBundle.GetCertificateChain())

	return newBundle, nil
}

// GenerateInstanceCertificate creates an X.509 certificate and encrypted private
// key for a new instance.
//
// The admin identified by certRequest.AdminEmail must sign the instance
// certificate. The admin's private key is retrieved by decrypting
// adminCertBundle.GetEncryptedKey() using a password derived from adminPassword
// and certRequest.AdminEmail. Only admin user certificates are supported as
// signers - root CA certificates are not allowed to sign instance certificates directly.
//
// A random instance UUID is generated and used as the certificate CommonName.
// A random instance password is generated and used to derive the encryption key
// for the instance's private key. The returned string is this instance password;
// it is required to decrypt the private key stored in the returned bundle.
//
// Parameters:
//   - certRequest: contains the target Organization and AdminEmail (signer).
//   - adminCertBundle: certificate and encrypted private key for the admin or CA.
//   - adminPassword: passphrase used (with AdminEmail) to derive the admin key
//     decryption password.
//
// Returns:
//   - CertificateBundle: the instance certificate and encrypted private key.
//   - string: the randomly generated instance password needed to decrypt the
//     private key in the bundle.
//   - error: non-nil if any step (admin key decode, certificate generation,
//     private key encryption) fails.

func (g *GoCrypto) GenerateInstanceCertificate(certRequest InstanceCertRequest, adminCertBundle CertificateBundle, adminPassword string) (CertificateBundle, string, error) {
	adminCert := adminCertBundle.GetCertificate()
	adminEncryptedKey := adminCertBundle.GetEncryptedKey()

	if adminCert == nil {
		return CertificateBundle{}, "", errors.New("admin certificate is nil")
	}

	// Use the provided admin email for password derivation
	adminEmail := certRequest.AdminEmail
	if adminEmail == "" {
		return CertificateBundle{}, "", errors.New("admin email is required")
	}

	// Validate signer identity and privileges
	if adminCert.Subject.CommonName != adminEmail {
		return CertificateBundle{}, "", errors.New("admin email does not match admin certificate CommonName")
	}
	if role, err := certificate.GetRoleFromCertificate(adminCert); err != nil || role != certificate.RoleAdmin {
		return CertificateBundle{}, "", errors.New("admin certificate must have Admin role")
	}
	if !adminCert.IsCA || (adminCert.KeyUsage&x509.KeyUsageCertSign == 0) {
		return CertificateBundle{}, "", errors.New("admin certificate is not CA-capable (IsCA/KeyUsageCertSign required)")
	}
	if certRequest.Organization == "" {
		return CertificateBundle{}, "", errors.New("organization is required")
	}

	// Derive the admin key password - only admin user certificates are allowed to sign certificates
	adminKeyPassword := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(adminPassword, adminEmail)

	// Decode the admin private key
	adminPrivateKey, err := encoder.DecodeEd25519PrivateKey(adminEncryptedKey, adminKeyPassword)
	if err != nil {
		return CertificateBundle{}, "", errors.Join(err, errors.New("failed to decode admin private key"))
	}

	// Generate a random password for the instance certificate
	instancePasswordBytes := make([]byte, 32)
	_, err = rand.Read(instancePasswordBytes)
	if err != nil {
		return CertificateBundle{}, "", errors.Join(err, errors.New("failed to generate random instance password"))
	}
	instancePassword := hex.EncodeToString(instancePasswordBytes)

	// Generate a unique instance UUID
	instanceUUIDBytes := make([]byte, 16)
	_, err = rand.Read(instanceUUIDBytes)
	if err != nil {
		return CertificateBundle{}, "", errors.Join(err, errors.New("failed to generate random instance UUID"))
	}
	instanceUUID := hex.EncodeToString(instanceUUIDBytes)

	// Generate the instance certificate and private key
	instanceCert, instancePrivateKey, err := certificate.GenerateX509InstanceCertificateAndKey(
		certificate.InstanceRequest{
			Organization: certRequest.Organization,
			CommonName:   instanceUUID,
			DNSNames:     []string{},
			IPAddresses:  []net.IP{},
		},
		adminCert,
		adminPrivateKey,
	)
	if err != nil {
		return CertificateBundle{}, "", errors.Join(err, errors.New("failed to generate instance certificate"))
	}

	// Derive password for instance key encryption
	instanceKeyPassword := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(instancePassword, instanceUUID)

	// Encrypt and encode the instance private key
	instanceEncryptedKey, err := encoder.EncodeEd25519PrivateKey(instancePrivateKey, instanceKeyPassword)
	if err != nil {
		return CertificateBundle{}, "", errors.Join(err, errors.New("failed to encrypt instance private key"))
	}

	// Construct the certificate chain
	// Chain order: [direct issuer (admin), ... intermediates ..., owner, (root CA not included)]
	// Admin cert first (direct issuer), then admin's chain in reverse order
	// User chains are stored bottom-up [root-adjacent, ..., direct-signer]
	// Instance chains need top-down [direct-signer, ..., root-adjacent]
	adminChain := adminCertBundle.GetCertificateChain()
	instanceChain := []string{encoder.EncodeX509Certificate(adminCert)}
	// Reverse append the admin's chain to maintain correct order
	for i := len(adminChain) - 1; i >= 0; i-- {
		instanceChain = append(instanceChain, adminChain[i])
	}

	// Create instance certificate bundle with chain
	instanceBundle := NewCertificateBundleWithChain(instanceCert, instanceEncryptedKey, instanceChain)

	return instanceBundle, instancePassword, nil
}

// GenerateInstanceCertificateV2 creates an X.509 certificate and encrypted private
// key for a new instance using the V2 certificate generation method.
//
// This method uses the Phase 1 GenerateX509InstanceCertificateAndKeyV2 function
// which enables admin certificates to sign instance certificates directly
// (rather than requiring root CA signing as in V1).
//
// The admin identified by certRequest.AdminEmail must sign the instance
// certificate. The admin's private key is retrieved by decrypting
// adminCertBundle.GetEncryptedKey() using a password derived from adminPassword
// and certRequest.AdminEmail. Only admin user certificates are supported as
// signers - root CA certificates are not allowed to sign instance certificates directly.
//
// The instanceAuthToken (from certRequest) is hashed once with SHA3-256 and used
// to derive the encryption key for the instance's private key. This matches V1 behavior
// where the instance can decrypt its certificate using sha3_256(instanceAuthToken).
//
// The instanceUUID (from certRequest) is used as the certificate CommonName.
//
// Parameters:
//   - certRequest: contains Organization, AdminEmail (signer), InstanceAuthToken, and InstanceUUID.
//   - adminCertBundle: certificate and encrypted private key for the admin.
//   - adminDerivedPassword: already-derived password (from raw password + AdminEmail)
//     used to decrypt the admin private key.
//
// Returns:
//   - CertificateBundle: the instance certificate and encrypted private key.
//   - string: the derived password used to encrypt the instance private key (for validation).
//     The instance will derive this independently from sha3_256(instanceAuthToken) + instanceUUID.
//   - error: non-nil if any step (admin key decode, certificate generation,
//     private key encryption) fails.
func (g *GoCrypto) GenerateInstanceCertificateV2(certRequest InstanceCertRequest, adminCertBundle CertificateBundle, adminDerivedPassword string) (CertificateBundle, string, error) {
	adminCert := adminCertBundle.GetCertificate()
	adminEncryptedKey := adminCertBundle.GetEncryptedKey()

	if adminCert == nil {
		return CertificateBundle{}, "", errors.New("admin certificate is nil")
	}

	// Early validation for all required fields
	if certRequest.AdminEmail == "" {
		return CertificateBundle{}, "", errors.New("admin email is required")
	}
	if certRequest.Organization == "" {
		return CertificateBundle{}, "", errors.New("organization is required")
	}
	if certRequest.InstanceAuthToken == "" {
		return CertificateBundle{}, "", errors.New("instance auth token is required")
	}
	if certRequest.InstanceUUID == "" {
		return CertificateBundle{}, "", errors.New("instance UUID is required")
	}

	// Use the provided admin email for password derivation
	adminEmail := certRequest.AdminEmail

	// Validate signer identity and privileges
	if adminCert.Subject.CommonName != adminEmail {
		return CertificateBundle{}, "", errors.New("admin email does not match admin certificate CommonName")
	}
	if role, err := certificate.GetRoleFromCertificate(adminCert); err != nil || role != certificate.RoleAdmin {
		return CertificateBundle{}, "", errors.New("admin certificate must have Admin role")
	}
	if !adminCert.IsCA || (adminCert.KeyUsage&x509.KeyUsageCertSign == 0) {
		return CertificateBundle{}, "", errors.New("admin certificate is not CA-capable (IsCA/KeyUsageCertSign required)")
	}
	// Decode the admin private key using the derived password
	adminPrivateKey, err := encoder.DecodeEd25519PrivateKey(adminEncryptedKey, adminDerivedPassword)
	if err != nil {
		return CertificateBundle{}, "", errors.Join(err, errors.New("failed to decode admin private key"))
	}

	// Use the provided instanceAuthToken (hashed once with SHA3-256) as the instance password
	// This matches V1 behavior where: 1x hash = encryption, 2x hash = authentication
	hasher := sha3.New256()
	hasher.Write([]byte(certRequest.InstanceAuthToken))
	instancePasswordHash := hasher.Sum(nil)
	instancePassword := hex.EncodeToString(instancePasswordHash)

	// Use the provided instance UUID
	instanceUUID := certRequest.InstanceUUID

	// Generate the instance certificate and private key using V2 method
	instanceCert, instancePrivateKey, err := certificate.GenerateX509InstanceCertificateAndKeyV2(
		certificate.InstanceRequest{
			Organization: certRequest.Organization,
			CommonName:   instanceUUID,
			DNSNames:     []string{},
			IPAddresses:  []net.IP{},
		},
		adminCert,
		adminPrivateKey,
	)
	if err != nil {
		return CertificateBundle{}, "", errors.Join(err, errors.New("failed to generate instance certificate"))
	}

	// Derive password for instance key encryption
	instanceKeyPassword := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(instancePassword, instanceUUID)

	// Encrypt and encode the instance private key
	instanceEncryptedKey, err := encoder.EncodeEd25519PrivateKey(instancePrivateKey, instanceKeyPassword)
	if err != nil {
		return CertificateBundle{}, "", errors.Join(err, errors.New("failed to encrypt instance private key"))
	}

	// Construct the certificate chain
	// Chain order: [direct issuer (admin), ... intermediates ..., owner, (root CA not included)]
	// Admin cert first (direct issuer), then admin's chain in reverse order
	// User chains are stored bottom-up [root-adjacent, ..., direct-signer]
	// Instance chains need top-down [direct-signer, ..., root-adjacent]
	adminChain := adminCertBundle.GetCertificateChain()
	instanceChain := []string{encoder.EncodeX509Certificate(adminCert)}
	// Reverse append the admin's chain to maintain correct order
	for i := len(adminChain) - 1; i >= 0; i-- {
		instanceChain = append(instanceChain, adminChain[i])
	}

	// Create instance certificate bundle with chain
	instanceBundle := NewCertificateBundleWithChain(instanceCert, instanceEncryptedKey, instanceChain)

	// Return instanceKeyPassword (the derived encryption password) for validation purposes
	// The instance will derive this independently from the raw instanceAuthToken
	return instanceBundle, instanceKeyPassword, nil
}

// GetCertificateVersion determines the version of a certificate based on its extensions.
//
// This function examines the certificate's extensions to determine whether it's
// a v1 or v2 certificate. v2 certificates contain location-role extensions that
// define permissions at specific locations, while v1 certificates use the older
// hierarchy-based permission system.
//
// Parameters:
//   - certBundle: the certificate bundle containing the certificate to examine.
//     The encrypted private key is not used for version detection.
//
// Returns:
//   - string: "v2" if the certificate contains location-role extensions, "v1"
//     for certificates using the legacy hierarchy system.
//   - error: non-nil if the certificate in the bundle is nil.
func (g *GoCrypto) GetCertificateVersion(certBundle CertificateBundle) (string, error) {
	cert := certBundle.GetCertificate()
	if cert == nil {
		return "", errors.New("certificate is nil")
	}

	if certificate.ExistsLocationRoleExtension(cert) {
		return "v2", nil
	}

	return "v1", nil
}

// GetUserRoles extracts the location-role mappings from a v2 certificate.
//
// This function examines the certificate's location-role extensions to extract the
// permissions for different locations. It only works with v2 certificates that contain
// location-role extensions; v1 certificates will return an error.
//
// Parameters:
//   - certBundle: the certificate bundle containing the certificate to examine.
//     The encrypted private key is not used for role extraction.
//
// Returns:
//   - certificate.LocationRoles: A map of location strings to roles (Admin, Editor, Viewer)
//   - error: non-nil if the certificate is nil or doesn't contain location-role extensions
func (g *GoCrypto) GetUserRoles(certBundle CertificateBundle) (certificate.LocationRoles, error) {
	cert := certBundle.GetCertificate()
	if cert == nil {
		return certificate.LocationRoles{}, errors.New("certificate is nil")
	}

	return certificate.GetLocationRolesFromCertificate(cert)
}

// IsInviteAllowedForLocation checks if the inviter certificate has permission to invite users at the specified location.
//
// This function determines whether the certificate holder has sufficient privileges to invite
// other users at the given location. It supports both V1 and V2 certificates with automatic
// version detection and uses hierarchical role resolution.
//
// Parameters:
//   - inviterCert: X.509 certificate of the user attempting to invite
//   - locationStr: Location string to check permission for (e.g., "umh.cologne.factory")
//
// Returns:
//   - error: nil if invite is allowed, error otherwise
//
// Permission Logic:
//   - Admin role: Can invite at any location they have access to
//   - Editor/Viewer roles: Cannot invite users (will return error)
//   - No access to location: Returns error
//   - V2 certificates: Uses LocationRoles extension with hierarchical lookup
//   - V1 certificates: fail - for a v1 company, only the CA can invite
func (g *GoCrypto) IsInviteAllowedForLocation(inviterCert *x509.Certificate, locationStr string) error {
	if inviterCert == nil {
		return errors.New("inviter certificate is nil")
	}

	// Get the role for the location
	role, err := certificate.GetRoleForLocation(inviterCert, locationStr)
	if err != nil {
		return fmt.Errorf("failed to get role for location: %w", err)
	}

	// Only Admin role can invite users
	if role != certificate.RoleAdmin {
		if role == "" {
			return errors.New("no access to location")
		}
		return fmt.Errorf("insufficient privileges: role %s cannot invite users (admin required)", role)
	}

	return nil
}

// EncryptRootCA encrypts a root CA certificate for secure storage.
//
// This function encrypts the root CA certificate using Argon2id key derivation and AES-256-GCM.
// The encryption key is derived from the keyMaterial and salt using the same derivation
// function used for private key encryption (DerivePrivateKeyEncryptionKeyFromPassword).
//
// Parameters:
//   - caCertPEM: The root CA certificate string (base64-encoded DER format)
//   - keyMaterial: The secret used for encryption:
//   - For users: their passphrase
//   - For invites: the invite key
//   - For instances: sha3_256(AUTH_TOKEN)
//   - salt: Context-specific salt for key derivation:
//   - For users: their email address
//   - For instances: their instanceUUID
//
// Returns:
//   - string: The encrypted certificate (base64-encoded JSON containing salt, nonce, ciphertext)
//   - error: non-nil if encryption fails
func (g *GoCrypto) EncryptRootCA(caCertPEM string, keyMaterial string, salt string) (string, error) {
	if caCertPEM == "" {
		return "", errors.New("CA certificate is empty")
	}
	if keyMaterial == "" {
		return "", errors.New("key material is empty")
	}
	if salt == "" {
		return "", errors.New("salt is empty")
	}

	// Derive the encryption key using the same derivation as private keys
	derivedKey := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(keyMaterial, salt)

	// Encrypt the certificate
	encrypted, err := encoder.EncryptCertificate(caCertPEM, derivedKey)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encrypt root CA"))
	}

	return encrypted, nil
}

// DecryptRootCA decrypts an encrypted root CA certificate.
//
// This function decrypts a root CA certificate that was encrypted using EncryptRootCA.
// The decryption key is derived from the keyMaterial and salt using the same derivation
// function used during encryption.
//
// Parameters:
//   - encryptedCA: The encrypted certificate string (base64-encoded JSON)
//   - keyMaterial: The secret used for decryption (must match what was used for encryption):
//   - For users: their passphrase
//   - For invites: the invite key
//   - For instances: sha3_256(AUTH_TOKEN)
//   - salt: Context-specific salt for key derivation (must match what was used for encryption):
//   - For users: their email address
//   - For instances: their instanceUUID
//
// Returns:
//   - string: The decrypted CA certificate (base64-encoded DER format)
//   - error: non-nil if decryption fails (wrong key, corrupted data, etc.)
func (g *GoCrypto) DecryptRootCA(encryptedCA string, keyMaterial string, salt string) (string, error) {
	if encryptedCA == "" {
		return "", errors.New("encrypted CA is empty")
	}
	if keyMaterial == "" {
		return "", errors.New("key material is empty")
	}
	if salt == "" {
		return "", errors.New("salt is empty")
	}

	// Derive the decryption key using the same derivation as private keys
	derivedKey := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(keyMaterial, salt)

	// Decrypt the certificate
	decrypted, err := encoder.DecryptCertificate(encryptedCA, derivedKey)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to decrypt root CA"))
	}

	return decrypted, nil
}
