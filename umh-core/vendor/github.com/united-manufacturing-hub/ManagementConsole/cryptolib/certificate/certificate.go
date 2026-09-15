// Package certificate implements the V2 certificate system with LocationRoles extension
// for the United Manufacturing Hub (UMH) management console.
//
// # System Configuration
//
// All system limitations, cryptographic parameters, and configuration values are
// centrally defined in constants.go. This includes:
//
// - Certificate validity periods and chain depth limits
// - Cryptographic algorithm specifications and key sizes
// - Role and permission system constraints
// - Location path format specifications
// - Password and key derivation parameters
// - Certificate extension size limits and security enforcement
//
// # Extension Size Limits
//
// The certificate system enforces size limits on all certificate extensions to prevent
// denial-of-service attacks and maintain reasonable certificate sizes:
//
// - CertificateExtensionSizeLimit: 16384 bytes (16 KB) per extension
// - Enforcement during certificate generation and parsing
// - Clear error messages and event logging for violations
// - Configurable limit for deployment-specific requirements
//
// See constants.go for detailed documentation of each limitation and its reasoning.
package certificate

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"time"
)

// CARequest represents a request to generate a Certificate Authority certificate.
// This is used to create the root CA that will sign all other certificates in the system.
type CARequest struct {
	Organization string // Organization name for the CA certificate
}

// GenerateX509CACertificateAndKey generates a new V1 Certificate Authority certificate and private key.
// This creates a self-signed V1 CA certificate with standard CA capabilities including CertSign and CRLSign key usage.
// V1 CA certificates do not include LocationRole extensions and are compatible with legacy V1 certificate systems.
//
// For V2 certificate systems that require LocationRole extensions, use GenerateX509CACertificateAndKeyV2.
//
// Parameters:
//   - req: CARequest containing the organization name for the certificate
//
// Returns:
//   - *x509.Certificate: The generated V1 CA certificate
//   - ed25519.PrivateKey: The CA private key for signing other certificates
//   - error: Any error that occurred during generation
func GenerateX509CACertificateAndKey(req CARequest) (*x509.Certificate, ed25519.PrivateKey, error) {
	// Generate Ed25519 key pair
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to generate Ed25519 key pair"))
	}

	// Prepare certificate template
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to generate serial number"))
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{req.Organization},
		},
		NotBefore:             time.Now().Add(-CertificateClockSkewTolerance),
		NotAfter:              time.Now().AddDate(CertificateValidityYears, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            MaxCertificatePathLength,
	}

	// V1 CA certificates do not include LocationRole extensions
	// This ensures compatibility with legacy V1 certificate systems

	// Create self-signed certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, pub, priv)
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to create self-signed certificate"))
	}

	// Parse the certificate to return as *x509.Certificate
	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to parse certificate"))
	}

	return cert, priv, nil
}

// GenerateX509CACertificateAndKeyV2 generates a new V2 Certificate Authority certificate and private key.
// The CA certificate has admin permissions for all locations (*) and can sign other certificates.
// This creates a self-signed V2 CA certificate with CA capabilities including CertSign and CRLSign key usage,
// and includes the LocationRole extension required for V2 certificate chains.
//
// V2 CA certificates include LocationRole extensions and enable the V2 permission system with unlimited
// location hierarchy depth and granular location-based role mapping.
//
// Parameters:
//   - req: CARequest containing the organization name for the certificate
//
// Returns:
//   - *x509.Certificate: The generated V2 CA certificate with LocationRole extension
//   - ed25519.PrivateKey: The CA private key for signing other certificates
//   - error: Any error that occurred during generation
func GenerateX509CACertificateAndKeyV2(req CARequest) (*x509.Certificate, ed25519.PrivateKey, error) {
	// Generate Ed25519 key pair
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to generate Ed25519 key pair"))
	}

	// Prepare certificate template
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to generate serial number"))
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{req.Organization},
		},
		NotBefore:             time.Now().Add(-CertificateClockSkewTolerance),
		NotAfter:              time.Now().AddDate(CertificateValidityYears, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            MaxCertificatePathLength,
	}

	// Add V2 LocationRole extension for admin permissions to all locations
	locationRoles := LocationRoles{
		"*": RoleAdmin,
	}
	err = AddLocationRoleExtension(template, locationRoles)
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to add LocationRole extension"))
	}

	// Create self-signed certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, pub, priv)
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to create self-signed certificate"))
	}

	// Parse the certificate to return as *x509.Certificate
	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to parse certificate"))
	}

	return cert, priv, nil
}

// CertificateRequest represents a V1 certificate request with role and hierarchies.
// This is the legacy certificate request structure limited to 5-layer hierarchy depth.
// Use CertificateRequestV2 for new implementations with unlimited location depth.
type CertificateRequest struct {
	Organization string              // Organization name for the certificate
	CommonName   string              // Common name (usually user email or instance FQDN)
	Role         Role                // Certificate-level role (Admin, Editor, or Viewer)
	Hierarchies  []LocationHierarchy // V1 location hierarchies with 5-layer limit
}

// CertificateRequestV2 represents a V2 certificate request with location-based role mapping.
// Role and Hierarchies are automatically generated from LocationRoles via helper.generateLegacyExtensionsFromLocationRoles() for backwards compatibility.
type CertificateRequestV2 struct {
	Organization  string              // Certificate organization field
	CommonName    string              // Certificate common name field - the email of the user
	Role          Role                // Certificate-level role (backwards compatibility for V1/UMH Classic)
	Hierarchies   []LocationHierarchy // V1-style location hierarchies (backwards compatibility for V1/UMH Classic)
	LocationRoles LocationRoles       // Primary V2 feature: maps locations to specific roles (e.g., "enterprise.site" → RoleAdmin)
}

// GenerateX509UserCertificateAndKey generates a V1 user certificate with role and location hierarchies.
// This is the legacy certificate generation function limited to 5-layer location hierarchy.
// For new implementations, use GenerateX509UserCertificateAndKeyV2 with LocationRoles support.
//
// The generated certificate includes:
// - Role extension for V1 compatibility
// - Location extension with 5-layer hierarchies
// - Client authentication capabilities
//
// Parameters:
//   - req: CertificateRequest containing organization, common name, role, and hierarchies
//   - caCert: CA certificate for signing the user certificate
//   - caKey: CA private key for signing
//
// Returns:
//   - *x509.Certificate: The generated user certificate
//   - ed25519.PrivateKey: The user's private key
//   - error: Any error that occurred during generation
func GenerateX509UserCertificateAndKey(req CertificateRequest, caCert *x509.Certificate, caKey ed25519.PrivateKey) (*x509.Certificate, ed25519.PrivateKey, error) {
	// Generate Ed25519 key pair for the user
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to generate Ed25519 key pair"))
	}

	// Prepare certificate template
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to generate serial number"))
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{req.Organization},
			CommonName:   req.CommonName,
		},
		NotBefore:             time.Now().Add(-CertificateClockSkewTolerance),
		NotAfter:              time.Now().AddDate(CertificateValidityYears, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	// Validate and normalize the role using centralized parseRole function
	validatedRole, err := parseRole(req.Role)
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to validate role"))
	}
	req.Role = validatedRole

	// Add role extension
	if err := AddRoleExtension(template, req.Role); err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to add role extension"))
	}

	if len(req.Hierarchies) == 0 {
		// Add a default hierarchy that allows access to all locations
		req.Hierarchies = []LocationHierarchy{
			{
				Enterprise:     NewWildcardLocation(LocationTypeEnterprise),
				Site:           NewWildcardLocation(LocationTypeSite),
				Area:           NewWildcardLocation(LocationTypeArea),
				ProductionLine: NewWildcardLocation(LocationTypeProductionLine),
				WorkCell:       NewWildcardLocation(LocationTypeWorkCell),
			},
		}
	}

	// Add location extension
	if err := AddLocationExtension(template, req.Hierarchies); err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to add location extension"))
	}

	// Create certificate signed by the CA
	derBytes, err := x509.CreateCertificate(rand.Reader, template, caCert, pub, caKey)
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to create certificate signed by the CA"))
	}

	// Parse the certificate to return as *x509.Certificate
	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to parse certificate"))
	}

	return cert, priv, nil
}

// GenerateX509UserCertificateAndKeyV2 generates a V2 user certificate with location-based role mapping.
// This is the primary V2 certificate generation function supporting unlimited location hierarchy depth.
// It implements the core V2 certificate system with LocationRoles extension for granular permissions.
//
// The generated certificate includes:
// - LocationRoles extension mapping location paths to specific roles
// - Legacy Role and Location extensions for backward compatibility with UMH Classic
// - CA capabilities for Admin users (IsCA: true, KeyUsage: CertSign)
// - Client authentication capabilities
//
// Admin users receive CA capabilities enabling them to sign certificates for other users,
// implementing the admin delegation system described in ENG-3457.
//
// Parameters:
//   - req: CertificateRequestV2 containing organization, common name, and LocationRoles mapping
//   - caCert: CA certificate for signing the user certificate
//   - caKey: CA private key for signing
//
// Returns:
//   - *x509.Certificate: The generated V2 user certificate
//   - ed25519.PrivateKey: The user's private key
//   - error: Any error that occurred during generation
func GenerateX509UserCertificateAndKeyV2(req CertificateRequestV2, caCert *x509.Certificate, caKey ed25519.PrivateKey) (*x509.Certificate, ed25519.PrivateKey, error) {
	// Generate Ed25519 key pair for the user
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate Ed25519 key pair: %w", err)
	}

	// Prepare certificate template
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), SerialNumberBitLength)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{req.Organization},
			CommonName:   req.CommonName,
		},
		NotBefore:             time.Now().Add(-CertificateClockSkewTolerance),
		NotAfter:              time.Now().AddDate(CertificateValidityYears, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,                                              // Removed KeyEncipherment for Ed25519
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, // Both for API auth and intermediate CA capability
		BasicConstraintsValid: true,
		IsCA:                  false, // Default to non-CA
	}

	// Validate and normalize the role using centralized parseRole function
	validatedRole, err := parseRole(req.Role)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to validate role: %w", err)
	}
	req.Role = validatedRole

	if req.Role == RoleAdmin {
		// Only admin users should be CA-capable for delegation system
		template.IsCA = true
		template.MaxPathLen = MaxCertificatePathLength
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	// Add role extension (for backwards compatibility with UMH Classic)
	if err := AddRoleExtension(template, req.Role); err != nil {
		return nil, nil, fmt.Errorf("failed to add role extension: %w", err)
	}

	// Add locationRoles extension (for V2 certificate system)
	if err := AddLocationRoleExtension(template, req.LocationRoles); err != nil {
		return nil, nil, fmt.Errorf("failed to add LocationRoles extension: %w", err)
	}

	if len(req.Hierarchies) == 0 {
		// Add a default hierarchy that allows access to all locations
		req.Hierarchies = []LocationHierarchy{
			{
				Enterprise:     NewWildcardLocation(LocationTypeEnterprise),
				Site:           NewWildcardLocation(LocationTypeSite),
				Area:           NewWildcardLocation(LocationTypeArea),
				ProductionLine: NewWildcardLocation(LocationTypeProductionLine),
				WorkCell:       NewWildcardLocation(LocationTypeWorkCell),
			},
		}
	}

	// Add location extension (for backwards compatibility with UMH Classic)
	if err := AddLocationExtension(template, req.Hierarchies); err != nil {
		return nil, nil, fmt.Errorf("failed to add location extension: %w", err)
	}

	// Create certificate signed by the CA
	derBytes, err := x509.CreateCertificate(rand.Reader, template, caCert, pub, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate signed by the CA: %w", err)
	}

	// Parse the certificate to return as *x509.Certificate
	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Validate (this only validates the correctness of the signature from the issuer to this certificate, not the actual chain)
	err = ValidateX509UserCertificate(cert, caCert, []*x509.Certificate{})
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to validate generated certificate"))
	}

	return cert, priv, nil
}

// validateNoDuplicateExtensions ensures that a certificate does not contain
// multiple extensions with the same OID, as prohibited by RFC 5280.
//
// According to RFC 5280 Section 4.2: "A certificate MUST NOT include more
// than one instance of a particular extension."
//
// This function provides defense against malformed or malicious certificates
// that might attempt to bypass validation by including duplicate extensions.
//
// Parameters:
//   - cert: The X.509 certificate to validate
//
// Returns:
//   - error: nil if no duplicates found, error describing the duplicate extension
func validateNoDuplicateExtensions(cert *x509.Certificate) error {
	seen := make(map[string]bool)
	for _, ext := range cert.Extensions {
		oidStr := ext.Id.String()
		if seen[oidStr] {
			return fmt.Errorf("certificate contains duplicate extension with OID %s (RFC 5280 violation)", oidStr)
		}
		seen[oidStr] = true
	}
	return nil
}

// ValidateX509UserCertificate validates a user certificate against a certificate chain.
// This function performs comprehensive validation including certificate chain verification,
// role validation, location hierarchy validation, and V2 LocationRole extension validation.
//
// Validation steps performed:
// - RFC 5280 compliance check for duplicate extensions
// - Certificate chain verification against root and intermediate CAs
// - Role extension validation for valid user roles (Admin, Editor, Viewer)
// - Location hierarchy extension validation
// - LocationRole extension validation if present (V2 certificates)
//
// Trust Model: Only the self-signed root CA needs to be explicitly trusted.
// Intermediate certificates are validated as part of the chain verification process.
//
// Parameters:
//   - cert: The user certificate to validate
//   - rootCA: The trusted root CA certificate (self-signed)
//   - intermediates: Array of intermediate CA certificates (can be empty for direct CA signing)
//
// Returns:
//   - error: nil if validation passes, otherwise describes the validation failure
func ValidateX509UserCertificate(cert *x509.Certificate, rootCA *x509.Certificate, intermediates []*x509.Certificate) error {
	// Validate that the certificate does not contain duplicate extensions (RFC 5280 compliance)
	if err := validateNoDuplicateExtensions(cert); err != nil {
		return errors.Join(err, errors.New("certificate extension validation failed"))
	}

	// Create certificate pools
	// NOTE: Only the self-signed root CA is added to the trusted roots pool.
	// Intermediate certificates are validated as part of the chain verification process.
	roots := x509.NewCertPool()
	roots.AddCert(rootCA)

	intermediatePool := x509.NewCertPool()
	for _, intermediateCert := range intermediates {
		intermediatePool.AddCert(intermediateCert)
	}

	// Create verification options
	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediatePool,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	// Verify the certificate
	_, err := cert.Verify(opts)
	if err != nil {
		return errors.Join(err, errors.New("failed to verify certificate"))
	}

	// Verify role extension
	role, err := GetRoleFromCertificate(cert)
	if err != nil {
		return errors.Join(err, errors.New("failed to get role from certificate"))
	}

	// Verify locations extension
	_, err = GetLocationHierarchiesFromCertificate(cert)
	if err != nil {
		return errors.Join(err, errors.New("failed to get location hierarchies from certificate"))
	}

	// Validate the role using centralized parseRole function
	_, err = parseRole(role)
	if err != nil {
		return fmt.Errorf("invalid role for user certificate: %w", err)
	}

	// V2 SECURITY ENFORCEMENT: Check if any certificate in the chain is V2
	// If ANY certificate has LocationRole extension, enforce V2 rules for ALL certificates
	// This prevents downgrade attacks where V1 user certs are used in V2-capable chains
	isV2Chain := ExistsLocationRoleExtension(cert) || ExistsLocationRoleExtension(rootCA)
	for _, intermediateCert := range intermediates {
		if ExistsLocationRoleExtension(intermediateCert) {
			isV2Chain = true
			break
		}
	}

	// If this is a V2 chain, enforce that the user certificate has LocationRole extension
	if isV2Chain && !ExistsLocationRoleExtension(cert) {
		return errors.New("V2 certificate chain detected: user certificate must have LocationRole extension")
	}

	// If the certificate has a locationRole extension, verify it
	if err := VerifyLocationRole(cert, rootCA, intermediates); err != nil {
		return fmt.Errorf("invalid LocationRole extension: %s", err)
	}

	return nil
}

// VerifyLocationRole validates that the issuer had admin permissions for all locations
// granted in the LocationRole extension of the issued certificate.
// This enforces the security requirement that only admins can delegate permissions
// for locations they have admin access to.

// It does not cryptographically verify that this certificate is trusted.
// This has to be done in the caller.
//
// Parameters:
//   - cert: Certificate to validate LocationRole permissions for
//   - rootCA: The trusted root CA certificate
//   - intermediates: Array of intermediate CA certificates
//
// Returns:
//   - error: nil if validation passes, error if issuer lacks required permissions
//
// Helper functions used: validateCertificateChainRecursive
func VerifyLocationRole(cert *x509.Certificate, rootCA *x509.Certificate, intermediates []*x509.Certificate) error {
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(oidExtensionRoleLocation) {
			// Build and verify the correct certificate chain order
			orderedChain, err := buildOrderedCertificateChain(cert, rootCA, intermediates)
			if err != nil {
				return fmt.Errorf("failed to build ordered certificate chain: %w", err)
			}

			// Recursively validate the certificate chain
			return validateCertificateChainRecursive(cert, orderedChain, 0)
		}
	}

	return nil // locationRole extension not found
}

// buildOrderedCertificateChain builds a correctly ordered certificate chain from immediate issuer to root.
// It verifies the chain by matching Subject Key Identifier to Authority Key Identifier.
// Returns: [immediate_issuer, intermediate1, intermediate2, ..., root_CA]
// Helper functions used: isIssuer
func buildOrderedCertificateChain(cert *x509.Certificate, rootCA *x509.Certificate, intermediates []*x509.Certificate) ([]*x509.Certificate, error) {
	// All available certificates to build the chain from
	allCerts := make([]*x509.Certificate, 0, len(intermediates)+1)
	allCerts = append(allCerts, intermediates...)
	allCerts = append(allCerts, rootCA)

	var orderedChain []*x509.Certificate
	currentCert := cert

	// Build chain by following Authority Key Identifier → Subject Key Identifier links
	for len(allCerts) > 0 {
		// Find the issuer of the current certificate
		issuerFound := false

		for i, candidate := range allCerts {
			// Check if this candidate issued the current certificate
			if isIssuer(currentCert, candidate) {
				// Add to chain and remove from available certificates
				orderedChain = append(orderedChain, candidate)
				allCerts = append(allCerts[:i], allCerts[i+1:]...)
				currentCert = candidate
				issuerFound = true
				break
			}
		}

		if !issuerFound {
			if len(orderedChain) == 0 {
				return nil, fmt.Errorf("no issuer found for certificate %s", cert.Subject.CommonName)
			}
			// Reached the end of the chain (root CA has no issuer)
			break
		}
	}

	// Verify that the chain actually terminates at the root CA
	if len(orderedChain) == 0 {
		return nil, fmt.Errorf("failed to build certificate chain: no issuers found")
	}

	// The last certificate in the chain MUST be the root CA
	lastCert := orderedChain[len(orderedChain)-1]
	if !lastCert.Equal(rootCA) {
		return nil, fmt.Errorf("certificate chain does not terminate at trusted root CA: chain ends at %s instead of %s",
			lastCert.Subject.CommonName, rootCA.Subject.CommonName)
	}

	return orderedChain, nil
}

// isIssuer checks if candidateIssuer issued the given certificate by comparing key identifiers
func isIssuer(cert *x509.Certificate, candidateIssuer *x509.Certificate) bool {
	// Primary check: Authority Key Identifier of cert should match Subject Key Identifier of issuer
	if len(cert.AuthorityKeyId) > 0 && len(candidateIssuer.SubjectKeyId) > 0 {
		return string(cert.AuthorityKeyId) == string(candidateIssuer.SubjectKeyId)
	}

	// Fallback: Check if issuer's subject matches cert's issuer
	return cert.Issuer.String() == candidateIssuer.Subject.String()
}

// validateCertificateChainRecursive recursively validates that each certificate in the chain
// was issued by an authority that had admin rights for all locations in the issued certificate.
func validateCertificateChainRecursive(issuedCert *x509.Certificate, remainingChain []*x509.Certificate, depth int) error {
	// Check maximum chain depth to prevent DoS attacks and excessively long chains
	if depth >= MaxCertificatePathLength {
		return fmt.Errorf("certificate chain depth (%d) exceeds maximum allowed depth (%d)", depth, MaxCertificatePathLength)
	}

	// Base case: if no more issuers in chain, validation is complete
	if len(remainingChain) == 0 {
		return nil
	}

	// Get the immediate issuer (first cert in remaining chain)
	issuerCert := remainingChain[0]

	// Get all locations that the issued certificate claims access to
	certLocationRoles, err := GetLocationRolesFromCertificate(issuedCert)
	if err != nil {
		return fmt.Errorf("failed to get locationRoles from certificate %s: %w", issuedCert.Subject.CommonName, err)
	}

	// Verify that the issuer has admin rights for each location claimed by the issued certificate
	for location := range certLocationRoles {
		role, err := GetRoleForLocation(issuerCert, location)
		if err != nil {
			return fmt.Errorf("issuer %s lacks authority for location %s: %w", issuerCert.Subject.CommonName, location, err)
		}
		if role != RoleAdmin {
			return fmt.Errorf("issuer %s has insufficient permissions (%s) to grant admin access to location %s", issuerCert.Subject.CommonName, role, location)
		}
	}

	// Recursive case: validate the issuer certificate against its own issuer
	return validateCertificateChainRecursive(issuerCert, remainingChain[1:], depth+1)
}

// ExistsLocationRoleExtension checks if a certificate contains the V2 LocationRole extension.
// This is used for certificate version detection to determine if a certificate supports
// the V2 location-based role mapping system or uses the legacy V1 hierarchy system.
//
// Parameters:
//   - cert: Certificate to check for LocationRole extension
//
// Returns:
//   - bool: true if the certificate contains V2 LocationRole extension, false otherwise
func ExistsLocationRoleExtension(cert *x509.Certificate) bool {
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(oidExtensionRoleLocation) {
			return true
		}
	}
	return false
}

// InstanceRequest represents a request to generate an instance certificate for automated systems.
// Instance certificates are used for server authentication and typically include DNS names and IP addresses
// for the services they represent. These certificates do not include role or location extensions.
type InstanceRequest struct {
	Organization string   // Organization name for the certificate
	CommonName   string   // Usually the FQDN of the instance
	DNSNames     []string // Alternative DNS names for the instance
	IPAddresses  []net.IP // IP addresses the instance will use
}

// GetRoleForLocation performs hierarchical permission lookup to find the user's role for a specific location.
// This implements the core V2 permission resolution algorithm that supports unlimited hierarchy depth.
//
// The function searches from most specific to least specific location path:
// 1. Exact location match (e.g., "umh.cologne.factory.line1")
// 2. Parent location matches (e.g., "umh.cologne.factory", "umh.cologne", "umh")
// 3. Wildcard match ("*")
//
// This enables granular permission assignment where a user might have Editor access to
// "umh.cologne" but Admin access to "umh.cologne.factory.line1.station5".
//
// **Security**: Uses SafeParseLocationString to prevent injection attacks and validate
// location format before processing.
//
// Parameters:
//   - cert: Certificate containing LocationRoles extension
//   - location: Location path to check permissions for (dot-separated)
//
// Returns:
//   - Role: The role (Admin, Editor, Viewer) for the location
//   - error: Error if no role found for location or any parent locations
func GetRoleForLocation(cert *x509.Certificate, location string) (Role, error) {
	locationRoles, err := GetLocationRolesFromCertificate(cert)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to get locationRoles from certificate"))
	}

	// Safely parse and validate the location string
	segments, err := SafeParseLocationString(location)
	if err != nil {
		return "", errors.Join(err, errors.New("invalid location format"))
	}

	// Generate safe location candidates for hierarchical lookup
	candidates := SafeParseLocationCandidates(segments)

	for _, candidate := range candidates {
		if role, exists := locationRoles[candidate]; exists {
			return role, nil
		}
	}

	return "", errors.New("did not find role for location or any parent locations")
}

// GenerateX509InstanceCertificateAndKey generates a certificate for automated systems and services.
// Instance certificates are used for server authentication and include DNS names and IP addresses
// for TLS connections. These certificates do not include role or location extensions as they
// are not used for user permission validation.
//
// The generated certificate includes:
// - Server authentication capabilities (ExtKeyUsageServerAuth)
// - DNS names including the common name
// - IP addresses for direct IP-based connections
// - URI with the common name as host
//
// Parameters:
//   - req: InstanceRequest containing organization, common name, DNS names, and IP addresses
//   - caCert: CA certificate for signing the instance certificate
//   - caKey: CA private key for signing
//
// Returns:
//   - *x509.Certificate: The generated instance certificate
//   - ed25519.PrivateKey: The instance's private key
//   - error: Any error that occurred during generation
func GenerateX509InstanceCertificateAndKey(req InstanceRequest, caCert *x509.Certificate, caKey ed25519.PrivateKey) (*x509.Certificate, ed25519.PrivateKey, error) {
	// Generate Ed25519 key pair for the instance
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to generate Ed25519 key pair"))
	}

	// Prepare certificate template
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to generate serial number"))
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{req.Organization},
			CommonName:   req.CommonName,
		},
		NotBefore:             time.Now().Add(-CertificateClockSkewTolerance),
		NotAfter:              time.Now().AddDate(CertificateValidityYears, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature, // Removed KeyEncipherment for Ed25519
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              append(req.DNSNames, req.CommonName),
		IPAddresses:           req.IPAddresses,
		URIs:                  []*url.URL{&url.URL{Host: req.CommonName}},
	}

	// Create certificate signed by the CA
	derBytes, err := x509.CreateCertificate(rand.Reader, template, caCert, pub, caKey)
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to create certificate signed by the CA"))
	}

	// Parse the certificate to return as *x509.Certificate
	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, nil, errors.Join(err, errors.New("failed to parse certificate"))
	}

	return cert, priv, nil
}

// ValidateX509InstanceCertificate validates an instance certificate against a certificate chain.
// This function verifies that the instance certificate was properly signed by the CA
// and has the correct server authentication capabilities.
//
// Trust Model: Only the self-signed root CA needs to be explicitly trusted.
// Intermediate certificates are validated as part of the chain verification process.
//
// Parameters:
//   - cert: The instance certificate to validate
//   - rootCA: The trusted root CA certificate (self-signed)
//   - intermediates: Array of intermediate CA certificates (can be empty for direct CA signing)
//
// Returns:
//   - error: nil if validation passes, otherwise describes the validation failure
func ValidateX509InstanceCertificate(cert *x509.Certificate, rootCA *x509.Certificate, intermediates []*x509.Certificate) error {
	// Validate that the certificate does not contain duplicate extensions (RFC 5280 compliance)
	if err := validateNoDuplicateExtensions(cert); err != nil {
		return errors.Join(err, errors.New("certificate extension validation failed"))
	}

	// Create certificate pools
	// NOTE: Only the self-signed root CA is added to the trusted roots pool.
	// Intermediate certificates are validated as part of the chain verification process.
	roots := x509.NewCertPool()
	roots.AddCert(rootCA)

	intermediatePool := x509.NewCertPool()
	for _, intermediateCert := range intermediates {
		intermediatePool.AddCert(intermediateCert)
	}

	// Create verification options
	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediatePool,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	// Verify the certificate
	_, err := cert.Verify(opts)
	if err != nil {
		return errors.Join(err, errors.New("failed to verify certificate"))
	}

	return nil
}

// IsLocationAllowedByHierarchies checks if a location is permitted by the V1 hierarchies
// stored in a certificate. This function implements V1-style location validation where
// hierarchies define 5-level permission structures.
//
// The function checks if the requested location matches any of the certificate's
// hierarchies by comparing each level (Enterprise, Site, Area, ProductionLine, WorkCell).
// Wildcard values ("*") in the hierarchy match any corresponding location segment.
//
// Parameters:
//   - cert: X.509 certificate containing V1 location hierarchies
//   - locationStr: Location path to check (dot-separated format)
//
// Returns:
//   - bool: true if location is allowed by any hierarchy, false otherwise
//   - error: Error if hierarchies cannot be extracted or location format is invalid
func IsLocationAllowedByHierarchies(cert *x509.Certificate, locationStr string) (bool, error) {
	// Get V1 hierarchies from certificate
	hierarchies, err := GetLocationHierarchiesFromCertificate(cert)
	if err != nil {
		return false, fmt.Errorf("failed to get location hierarchies from certificate: %w", err)
	}

	// Parse and validate the requested location
	segments, err := SafeParseLocationString(locationStr)
	if err != nil {
		return false, fmt.Errorf("invalid location format: %w", err)
	}

	// Handle wildcard location - always allowed
	if len(segments.Segments) == 1 && segments.Segments[0] == LocationWildcard {
		return true, nil
	}

	// Pad segments to match the 5-level V1 hierarchy structure
	// Missing levels are treated as wildcards for matching purposes
	locationParts := make([]string, 5)
	for i := 0; i < 5; i++ {
		if i < len(segments.Segments) {
			locationParts[i] = segments.Segments[i]
		} else {
			locationParts[i] = LocationWildcard // Missing levels match any hierarchy level
		}
	}

	// Check if any hierarchy allows this location
	for _, hierarchy := range hierarchies {
		if hierarchyAllowsLocation(hierarchy, locationParts) {
			return true, nil
		}
	}

	return false, nil
}

// hierarchyAllowsLocation checks if a single V1 hierarchy allows the given location parts.
// This implements the V1 matching logic where wildcards in the hierarchy match any location value.
func hierarchyAllowsLocation(hierarchy LocationHierarchy, locationParts []string) bool {
	hierarchyLevels := []Location{
		hierarchy.Enterprise,
		hierarchy.Site,
		hierarchy.Area,
		hierarchy.ProductionLine,
		hierarchy.WorkCell,
	}

	for i := 0; i < 5; i++ {
		hierarchyValue := hierarchyLevels[i].Value
		locationValue := locationParts[i]

		// Wildcard in hierarchy matches any location value
		if hierarchyValue == LocationWildcard {
			continue
		}

		// Wildcard in location matches any hierarchy value (for shorter location paths)
		if locationValue == LocationWildcard {
			continue
		}

		// Exact match required
		if hierarchyValue != locationValue {
			return false
		}
	}

	return true
}

// GenerateX509InstanceCertificateAndKeyV2 generates a V2 instance certificate signed by an admin certificate.
// This enables V2 certificate support where admins can sign instance certificates (vs V1 where only root CA signs).
// The admin certificate must have Admin role to sign instance certificates.
//
// The generated certificate includes:
// - Server authentication capabilities (ExtKeyUsageServerAuth)
// - DNS names including the common name
// - IP addresses for direct IP-based connections
// - URI with the common name as host
//
// Parameters:
//   - req: InstanceRequest containing organization, common name, DNS names, and IP addresses
//   - adminCert: Admin certificate for signing the instance certificate (must have Admin role)
//   - adminKey: Admin private key for signing
//
// Returns:
//   - *x509.Certificate: The generated instance certificate
//   - ed25519.PrivateKey: The instance's private key
//   - error: Any error that occurred during generation
func GenerateX509InstanceCertificateAndKeyV2(req InstanceRequest, adminCert *x509.Certificate, adminKey ed25519.PrivateKey) (*x509.Certificate, ed25519.PrivateKey, error) {
	// Validate that admin certificate has Admin role
	role, err := GetRoleFromCertificate(adminCert)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get role from admin certificate: %w", err)
	}
	if role != RoleAdmin {
		return nil, nil, fmt.Errorf("admin role required to sign instance certificates, got: %s", role)
	}

	// Validate that admin certificate is CA-capable
	if !adminCert.IsCA {
		return nil, nil, fmt.Errorf("admin certificate must be a CA with KeyUsageCertSign (IsCA=false)")
	}
	if adminCert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return nil, nil, fmt.Errorf("admin certificate must be a CA with KeyUsageCertSign (KeyUsageCertSign not set)")
	}

	// Generate Ed25519 key pair for the instance
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate Ed25519 key pair: %w", err)
	}

	// Prepare certificate template
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), SerialNumberBitLength)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{req.Organization},
			CommonName:   req.CommonName,
		},
		NotBefore:             time.Now().Add(-CertificateClockSkewTolerance),
		NotAfter:              time.Now().AddDate(CertificateValidityYears, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature, // Removed KeyEncipherment for Ed25519
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              append(req.DNSNames, req.CommonName),
		IPAddresses:           req.IPAddresses,
		URIs:                  []*url.URL{&url.URL{Host: req.CommonName}},
	}

	// Create certificate signed by the admin (not root CA)
	derBytes, err := x509.CreateCertificate(rand.Reader, template, adminCert, pub, adminKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate signed by admin: %w", err)
	}

	// Parse the certificate to return as *x509.Certificate
	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert, priv, nil
}

// GetPublicKeyFromCertificate extracts the Ed25519 public key from an X.509 certificate.
// This utility function is used in cryptographic operations that require the public key,
// such as key exchange or signature verification.
//
// Parameters:
//   - cert: X.509 certificate containing an Ed25519 public key
//
// Returns:
//   - ed25519.PublicKey: The extracted Ed25519 public key
//   - error: Error if the certificate doesn't contain a valid Ed25519 public key
func GetPublicKeyFromCertificate(cert *x509.Certificate) (ed25519.PublicKey, error) {
	pub, ok := cert.PublicKey.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("failed to get public key from certificate")
	}

	return pub, nil
}
