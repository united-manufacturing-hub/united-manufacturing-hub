// Package helper provides high-level cryptographic certificate management functions
// for the UMH certificate system.
package helper

import (
	"crypto/rand"
	"crypto/sha3"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/united-manufacturing-hub/ManagementConsole/cryptolib/certificate"
	"github.com/united-manufacturing-hub/ManagementConsole/cryptolib/encoder"
	"github.com/united-manufacturing-hub/ManagementConsole/cryptolib/password_derivation"
)

// GenerateCACertificateAndEncode generates a new Certificate Authority (CA) certificate
// and private key, then encodes them for storage and transmission.
//
// This function creates the root CA certificate that serves as the trust anchor for
// the entire certificate system. The CA certificate can sign both user certificates
// and admin certificates with delegation capabilities.
//
// Parameters:
//   - org: Organization name to include in the CA certificate
//   - userPassword: User's password for deriving the CA private key encryption password
//   - userEmail: User's email address used in password derivation
//
// Returns:
//   - string: JSON-encoded object containing:
//   - "certificate": PEM-encoded CA certificate
//   - "privateKey": Encrypted PEM-encoded CA private key
//   - "derivedPassword": Derived password used for private key encryption
//   - error: Any error that occurred during generation or encoding
//

func GenerateCACertificateAndEncode(org string, userPassword string, userEmail string) (string, error) {

	cert, privKey, err := certificate.GenerateX509CACertificateAndKey(certificate.CARequest{Organization: org})
	if err != nil {
		return "", errors.Join(err, errors.New("failed to generate CA"))
	}

	// Encode certificate and private key
	certStr := encoder.EncodeX509Certificate(cert)
	derivedPassword := password_derivation.DeriveCAEncryptionKeyFromPassword(userPassword, userEmail)
	keyStr, err := encoder.EncodeEd25519PrivateKey(privKey, derivedPassword)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encode CA private key"))
	}
	// Create return object
	result := make(map[string]interface{})
	result["certificate"] = certStr
	result["privateKey"] = keyStr
	result["derivedPassword"] = derivedPassword
	// JSON encode the result
	jsonResult, err := json.Marshal(result)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encode result"))
	}
	return string(jsonResult), nil
}

// same as above but generates a v2 certificate
func GenerateCACertificateAndEncodeV2(org string, userPassword string, userEmail string) (string, error) {

	cert, privKey, err := certificate.GenerateX509CACertificateAndKeyV2(certificate.CARequest{Organization: org})
	if err != nil {
		return "", errors.Join(err, errors.New("failed to generate CA"))
	}

	// Encode certificate and private key
	certStr := encoder.EncodeX509Certificate(cert)
	derivedPassword := password_derivation.DeriveCAEncryptionKeyFromPassword(userPassword, userEmail)
	keyStr, err := encoder.EncodeEd25519PrivateKey(privKey, derivedPassword)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encode CA private key"))
	}
	// Create return object
	result := make(map[string]interface{})
	result["certificate"] = certStr
	result["privateKey"] = keyStr
	result["derivedPassword"] = derivedPassword
	// JSON encode the result
	jsonResult, err := json.Marshal(result)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encode result"))
	}
	return string(jsonResult), nil
}

// GenerateUserOwnerCertificateAndEncode generates an owner user certificate with admin
// privileges for all locations using V1 certificate format.
//
// This function creates an owner certificate with full admin access across all locations
// using the legacy V1 certificate format for backward compatibility with existing systems.
//
// Parameters:
//   - org: Organization name for the certificate
//   - commonName: Common name (typically username) for the certificate
//   - caCertStr: PEM-encoded CA certificate string
//   - caKeyStr: Encrypted PEM-encoded CA private key string
//   - userPassword: User's password for private key encryption
//   - userEmail: User's email address for password derivation
//
// Returns:
//   - string: JSON-encoded certificate bundle (certificate, privateKey, derivedPassword)
//   - error: Any error that occurred during generation
//
// Note: This function generates V1 certificates for backward compatibility.
// For V2 certificates with enhanced features, use GenerateUserOwnerCertificateAndEncodeV2.
func GenerateUserOwnerCertificateAndEncode(org string, commonName string, caCertStr string, caKeyStr string, userPassword string, userEmail string) (string, error) {
	// Generate V1 owner certificate with Admin role and wildcard Enterprise location access
	// This provides backward compatibility with existing frontend and UMH Classic systems
	return GenerateUserOwnerCertificateAndEncodeWithRoleAndHierarchies(
		org,
		commonName,
		caCertStr,
		caKeyStr,
		userPassword,
		userEmail,
		userPassword,
		userEmail,
		certificate.RoleAdmin,
		[]certificate.LocationHierarchy{
			certificate.NewLocationHierarchy(
				certificate.NewWildcardLocation(certificate.LocationTypeEnterprise),
				certificate.NewWildcardLocation(certificate.LocationTypeSite),
				certificate.NewWildcardLocation(certificate.LocationTypeArea),
				certificate.NewWildcardLocation(certificate.LocationTypeProductionLine),
				certificate.NewWildcardLocation(certificate.LocationTypeWorkCell),
			),
		},
	)
}

// GenerateUserOwnerCertificateAndEncodeV2 generates an owner user certificate with admin
// privileges for all locations using V2 certificate format.
//
// This function creates an owner certificate with full admin access across all locations
// using the new V2 certificate format with enhanced features like LocationRoles.
//
// Parameters:
//   - org: Organization name for the certificate
//   - commonName: Common name (typically username) for the certificate
//   - caCertStr: PEM-encoded CA certificate string
//   - caKeyStr: Encrypted PEM-encoded CA private key string
//   - userPassword: User's password for private key encryption
//   - userEmail: User's email address for password derivation
//
// Returns:
//   - string: JSON-encoded certificate bundle (certificate, privateKey, derivedPassword)
//   - error: Any error that occurred during generation
//
// V2 Features:
//   - LocationRoles extension for unlimited hierarchy depth
//   - Enhanced security and flexibility
//   - Backward compatibility with V1 systems
func GenerateUserOwnerCertificateAndEncodeV2(org string, commonName string, caCertStr string, caKeyStr string, userPassword string, userEmail string) (string, error) {
	// Generate V2 owner certificate with Admin role and wildcard location access
	// Uses the new LocationRoles system for enhanced flexibility
	return GenerateUserOwnerCertificateAndEncodeWithRoleAndHierarchiesV2(
		org,
		commonName,
		caCertStr,
		caKeyStr,
		userPassword,
		userEmail,
		userPassword,
		userEmail,
		certificate.LocationRoles{
			"*": certificate.RoleAdmin,
		},
	)
}

// convertHierarchyToString converts a LocationHierarchy structure to its string
// representation using dot-separated notation.
//
// This helper function transforms the hierarchical location structure into a
// dot-separated string format used in the V2 certificate system. It handles
// wildcard locations by using "*" and builds the path from Enterprise down
// to WorkCell levels.
//
// Parameters:
//   - hierarchy: The LocationHierarchy to convert
//
// Returns:
//   - string: Dot-separated location string (e.g., "umh.cologne.factory.line1")
//
// Examples:
//   - Full wildcard: "*"
//   - Enterprise only: "umh"
//   - Enterprise + Site: "umh.cologne"
//   - Full path: "umh.cologne.factory.line1.station5"
//
// Note: The conversion stops at the first wildcard level encountered, as
// more specific levels after a wildcard are not meaningful.
func convertHierarchyToString(hierarchy certificate.LocationHierarchy) string {
	if hierarchy.Enterprise.IsWildcard() {
		return "*"
	}

	result := hierarchy.Enterprise.Value

	if hierarchy.Site.IsWildcard() {
		return result
	}
	result += "." + hierarchy.Site.Value

	if hierarchy.Area.IsWildcard() {
		return result
	}
	result += "." + hierarchy.Area.Value

	if hierarchy.ProductionLine.IsWildcard() {
		return result
	}
	result += "." + hierarchy.ProductionLine.Value

	if hierarchy.WorkCell.IsWildcard() {
		return result
	}
	result += "." + hierarchy.WorkCell.Value

	return result
}

// GenerateUserOwnerCertificateAndEncodeWithRoleAndHierarchies generates a V1 owner certificate
// with specified role and location hierarchies for backward compatibility.
//
// This function generates V1 certificates using the legacy certificate format that is
// compatible with the existing frontend and UMH Classic systems. It takes V1-style
// location hierarchies and applies the specified role for access control.
//
// # V1 Certificate Characteristics:
//
// - **Role-based Access**: Uses the specified role for the given hierarchies
// - **V1 Format**: Uses legacy certificate format and extensions
// - **Frontend Compatible**: Works with existing frontend certificate handling
// - **CA Capabilities**: Admin users get certificate signing capabilities (IsCA: true)
//
// Parameters:
//   - org: Organization name for the certificate
//   - commonName: Common name (typically user email) for the certificate
//   - caCertStr: PEM-encoded CA certificate string
//   - caKeyStr: Encrypted PEM-encoded CA private key string
//   - caUserPassword: Password for CA private key (direct or for derivation)
//   - caUserEmail: Email for CA password derivation if needed
//   - userPassword: User's password for private key encryption
//   - userEmail: User's email for password derivation
//   - role: Role to assign to the user for the specified hierarchies
//   - hierarchies: V1-style location hierarchies for access control
//
// Returns:
//   - string: JSON object containing:
//   - "certificate": PEM-encoded user certificate with V1 format
//   - "privateKey": Encrypted PEM-encoded user private key
//   - "derivedPassword": Password used for private key encryption
//   - error: Any error during certificate generation, encoding, or JSON marshaling
//
// # Example Usage:
//
//	hierarchies := []certificate.LocationHierarchy{
//		certificate.NewLocationHierarchy(
//			certificate.NewLocation(certificate.LocationTypeEnterprise, "umh"),
//			certificate.NewLocation(certificate.LocationTypeSite, "cologne"),
//			certificate.NewWildcardLocation(certificate.LocationTypeArea),
//			certificate.NewWildcardLocation(certificate.LocationTypeProductionLine),
//			certificate.NewWildcardLocation(certificate.LocationTypeWorkCell),
//		),
//	}
//
//	certBundle, err := GenerateUserOwnerCertificateAndEncodeWithRoleAndHierarchies(
//		"United Manufacturing Hub",
//		"admin@umh.app",
//		caCertPEM,
//		caKeyPEM,
//		caPassword,
//		"ca@umh.app",
//		userPassword,
//		"admin@umh.app",
//		certificate.RoleAdmin,
//		hierarchies,
//	)
//
// # Migration Note:
//
// TODO: Switch to V2 certificates once the frontend is ready to handle them.
// The frontend currently uses legacy functions that handle V1 certificates.
// Future implementations should use GenerateUserOwnerCertificateAndEncodeWithRoleAndHierarchiesV2
// for V2 LocationRoles support and enhanced capabilities.
func GenerateUserOwnerCertificateAndEncodeWithRoleAndHierarchies(
	org string,
	commonName string,
	caCertStr string,
	caKeyStr string,
	caUserPassword string,
	caUserEmail string,
	userPassword string,
	userEmail string,
	role certificate.Role,
	hierarchies []certificate.LocationHierarchy,
) (string, error) {
	// Validate that owner certificates must have Admin role
	if role != certificate.RoleAdmin {
		return "", fmt.Errorf("owner certificates must have Admin role, got: %s", role)
	}

	// Decode CA certificate and private key
	caCert, err := encoder.DecodeX509Certificate(caCertStr)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to decode CA certificate"))
	}

	caKey, err := encoder.DecodeEd25519PrivateKey(caKeyStr, caUserPassword) // This might already be the derived password
	if err != nil {
		// Derive caKeyPassword from userPassword
		caKeyPassword := password_derivation.DeriveCAEncryptionKeyFromPassword(caUserPassword, caUserEmail)
		// Try again
		caKey, err = encoder.DecodeEd25519PrivateKey(caKeyStr, caKeyPassword)
		if err != nil {
			return "", errors.Join(err, errors.New("failed to decode CA private key"))
		}
	}
	// Generate user certificate
	cert, privKey, err := certificate.GenerateX509UserCertificateAndKey(
		certificate.CertificateRequest{
			Organization: org,
			CommonName:   commonName,
			Role:         role,
			Hierarchies:  hierarchies,
		},
		caCert,
		caKey,
	)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to generate user certificate"))
	}
	// Encode certificate and private key
	certStr := encoder.EncodeX509Certificate(cert)
	derivedPassword := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(userPassword, userEmail)
	keyStr, err := encoder.EncodeEd25519PrivateKey(privKey, derivedPassword) // Note: We should probably add password protection
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encode user private key"))
	}
	// Create return object
	result := make(map[string]interface{})
	result["certificate"] = certStr
	result["privateKey"] = keyStr
	result["derivedPassword"] = derivedPassword
	// JSON encode the result
	jsonResult, err := json.Marshal(result)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encode result"))
	}
	return string(jsonResult), nil
}

// GenerateUserOwnerCertificateAndEncodeWithRoleAndHierarchiesV2 generates a V2
// certificate with LocationRoles extension for granular permission management.
//
// This is the primary function for creating V2 certificates that support unlimited
// hierarchy depth and location-specific role assignments. It generates both V2
// LocationRoles extensions and legacy V1 extensions for backward compatibility.
//
// Parameters:
//   - org: Organization name for the certificate
//   - commonName: Common name (typically username) for the certificate
//   - caCertStr: PEM-encoded CA certificate string
//   - caKeyStr: Encrypted PEM-encoded CA private key string
//   - caUserPassword: CA owner's password (or derived password)
//   - caUserEmail: CA owner's email address for password derivation
//   - userPassword: New user's password for private key encryption
//   - userEmail: New user's email address for password derivation
//   - locationRoles: Map of location strings to roles (e.g., {"umh.cologne": Admin})
//
// Returns:
//   - string: JSON-encoded certificate bundle with V2 LocationRoles extension
//   - error: Any error that occurred during generation
//
// LocationRoles Format:
//   - "*": Global access (all locations)
//   - "umh": Enterprise-level access
//   - "umh.cologne": Site-level access
//   - "umh.cologne.factory.line1": Production line access
//   - Support for unlimited hierarchy depth
//

func GenerateUserOwnerCertificateAndEncodeWithRoleAndHierarchiesV2(
	org string,
	commonName string,
	caCertStr string,
	caKeyStr string,
	caUserPassword string,
	caUserEmail string,
	userPassword string,
	userEmail string,
	locationRoles certificate.LocationRoles,
) (string, error) {
	// Decode CA certificate and private key
	caCert, err := encoder.DecodeX509Certificate(caCertStr)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to decode CA certificate"))
	}

	caKey, err := encoder.DecodeEd25519PrivateKey(caKeyStr, caUserPassword) // This might already be the derived password
	if err != nil {
		// Derive caKeyPassword from userPassword
		caKeyPassword := password_derivation.DeriveCAEncryptionKeyFromPassword(caUserPassword, caUserEmail)
		// Try again
		caKey, err = encoder.DecodeEd25519PrivateKey(caKeyStr, caKeyPassword)
		if err != nil {
			return "", errors.Join(err, errors.New("failed to decode CA private key"))
		}
	}

	hierarchies, role := generateLegacyExtensionsFromLocationRoles(locationRoles)

	// Generate user certificate
	cert, privKey, err := certificate.GenerateX509UserCertificateAndKeyV2(
		certificate.CertificateRequestV2{
			Organization:  org,
			CommonName:    commonName,
			Role:          role,
			Hierarchies:   hierarchies,
			LocationRoles: locationRoles,
		},
		caCert,
		caKey,
	)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to generate user certificate"))
	}
	// Encode certificate and private key
	certStr := encoder.EncodeX509Certificate(cert)
	derivedPassword := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(userPassword, userEmail)
	keyStr, err := encoder.EncodeEd25519PrivateKey(privKey, derivedPassword) // Note: We should probably add password protection
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encode user private key"))
	}
	// Create return object
	result := make(map[string]interface{})
	result["certificate"] = certStr
	result["privateKey"] = keyStr
	result["derivedPassword"] = derivedPassword
	// JSON encode the result
	jsonResult, err := json.Marshal(result)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encode result"))
	}
	return string(jsonResult), nil
}

// GetRoleForLocationFromString extracts the user's role for a specific location
// from a certificate string.
//
// This function decodes a PEM-encoded certificate and determines the user's
// role at the specified location using hierarchical role resolution. It supports
// both V1 and V2 certificates with automatic version detection.
//
// Parameters:
//   - certStr: PEM-encoded X.509 certificate string
//   - location: Location string to check role for (e.g., "umh.cologne.factory")
//
// Returns:
//   - certificate.Role: The role at the specified location (Admin, Editor, Viewer, or empty)
//   - error: Any error that occurred during decoding or role resolution
//
// Role Resolution:
//   - V2 certificates: Uses LocationRoles extension with hierarchical lookup
//   - V1 certificates: Uses legacy hierarchy extensions
//   - Finds the most specific location match for the given location
//   - Returns empty role if no matching location is found
//
// Examples:
//   - location "umh.cologne.factory" may match role at "umh.cologne" or "umh"
//   - Wildcard "*" location grants access to all locations
func GetRoleForLocationFromString(certStr string, location string) (certificate.Role, error) {
	// Decode certificate from PEM/string format
	cert, err := encoder.DecodeX509Certificate(certStr)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to decode certificate"))
	}

	return certificate.GetRoleForLocation(cert, location)
}

// GenerateUserCertificateAndEncodeWithRoleAndHierarchiesAndInviteKey generates
// a V1-style user certificate with a secure invite key for user onboarding.
//
// It is also used to generate invites
//
// This function implements the secure invite system by generating a random invite
// key that serves as the user's initial password. The invite key is hashed with
// SHA-3-256 for security and used to encrypt the user's private key.
//
// Parameters:
//   - org: Organization name for the certificate
//   - commonName: Common name (typically username) for the certificate
//   - caCertStr: PEM-encoded CA certificate string
//   - caKeyStr: Encrypted PEM-encoded CA private key string
//   - caKeyPassword: CA private key decryption password
//   - userEmail: New user's email address for password derivation
//   - role: Certificate-level role (Admin, Editor, Viewer)
//   - hierarchies: Array of location hierarchies with 5-level structure
//
// Returns:
//   - string: JSON-encoded object containing certificate, privateKey, derivedPassword, and inviteKey
//   - error: Any error that occurred during generation

// Deprecated: Use GenerateUserCertificateAndEncodeWithRoleAndHierarchiesAndInviteKeyV2
// for V2 certificate system with LocationRoles support.
func GenerateUserCertificateAndEncodeWithRoleAndHierarchiesAndInviteKey(org string, commonName string, caCertStr string, caKeyStr string, caKeyPassword string, userEmail string, role certificate.Role, hierarchies []certificate.LocationHierarchy) (string, error) {

	// Decode CA certificate and private key
	caCert, err := encoder.DecodeX509Certificate(caCertStr)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to decode CA certificate"))
	}

	caKey, err := encoder.DecodeEd25519PrivateKey(caKeyStr, caKeyPassword)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to decode CA private key"))
	}

	// Generate a random invite key (4096 bytes)
	inviteKeyMaterial := make([]byte, 4096)
	_, err = rand.Read(inviteKeyMaterial)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to generate random invite key"))
	}

	// Hash it with SHA-3-256 for sufficient security
	inviteKey := sha3.Sum256(inviteKeyMaterial)

	inviteKeyStr := hex.EncodeToString(inviteKey[:])

	// Generate user certificate
	cert, privKey, err := certificate.GenerateX509UserCertificateAndKey(
		certificate.CertificateRequest{
			Organization: org,
			CommonName:   commonName,
			Role:         role,
			Hierarchies:  hierarchies,
		},
		caCert,
		caKey,
	)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to generate user certificate"))
	}

	// Encode certificate and private key
	certStr := encoder.EncodeX509Certificate(cert)
	derivedPassword := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(inviteKeyStr, userEmail)
	keyStr, err := encoder.EncodeEd25519PrivateKey(privKey, derivedPassword)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encode user private key"))
	}

	// Create return object
	result := make(map[string]interface{})
	result["certificate"] = certStr
	result["privateKey"] = keyStr
	result["derivedPassword"] = derivedPassword
	result["inviteKey"] = inviteKeyStr

	// JSON encode the result
	jsonResult, err := json.Marshal(result)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encode result"))
	}
	return string(jsonResult), nil
}

// ExistsLocationRoleExtensionHelper checks if a certificate contains the V2
// LocationRoles extension for certificate version detection.
//
// This function is used to determine whether a certificate is V1 or V2 by
// checking for the presence of the LocationRoles extension (OID 1.3.6.1.4.1.59193.2.1).
// This enables automatic version detection and appropriate handling logic.
//
// Parameters:
//   - certStr: PEM-encoded X.509 certificate string
//
// Returns:
//   - bool: true if the certificate contains LocationRoles extension (V2), false otherwise (V1)
//   - error: Any error that occurred during certificate decoding
//
// Usage:
//   - Certificate version detection before role/permission extraction
//   - Routing logic to use appropriate V1 or V2 processing functions
//   - Migration planning and compatibility verification
//
// Note: V2 certificates contain both V2 LocationRoles and legacy V1 extensions
// for backward compatibility, so presence of this extension definitively
// identifies a V2 certificate.
func ExistsLocationRoleExtensionHelper(certStr string) (bool, error) {
	cert, err := encoder.DecodeX509Certificate(certStr)
	if err != nil {
		return false, errors.Join(err, errors.New("failed to decode certificate"))
	}
	exists := certificate.ExistsLocationRoleExtension(cert)
	return exists, nil
}

// GenerateUserCertificateAndEncodeWithRoleAndHierarchiesAndInviteKeyV2 generates
// a V2 certificate with LocationRoles extension and secure invite key for user onboarding.
//
// This is the primary function for admin-delegated certificate generation in the V2
// system. It allows admin users to create certificates for other users with specific
// location-based permissions and generates a secure invite key for initial access.
//
// Parameters:
//   - org: Organization name for the certificate
//   - commonName: Common name (typically username) for the certificate
//   - caCertStr: PEM-encoded CA certificate string
//   - caKeyStr: Encrypted PEM-encoded CA private key string
//   - caKeyPassword: CA private key decryption password
//   - userEmail: New user's email address for password derivation
//   - locationRoles: Map of location strings to roles for granular permissions
//
// Returns:
//   - string: JSON-encoded object containing certificate, privateKey, derivedPassword, and inviteKey
//   - error: Any error that occurred during generation
//
// Admin Delegation:
//   - Enables admin users to onboard new users with specific permissions
//   - Invite key serves as temporary password until user performs first login
//   - Supports granular location-based access control
func GenerateUserCertificateAndEncodeWithRoleAndHierarchiesAndInviteKeyV2(org string, commonName string, caCertStr string, caKeyStr string, caKeyPassword string, userEmail string, locationRoles certificate.LocationRoles) (string, error) {
	// Basic input validation
	if len(locationRoles) == 0 {
		return "", fmt.Errorf("locationRoles must not be empty")
	}

	// Decode CA certificate and private key
	caCert, err := encoder.DecodeX509Certificate(caCertStr)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to decode CA certificate"))
	}

	caKey, err := encoder.DecodeEd25519PrivateKey(caKeyStr, caKeyPassword)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to decode CA private key"))
	}

	// Generate a random invite key (4096 bytes)
	inviteKeyMaterial := make([]byte, 4096)
	_, err = rand.Read(inviteKeyMaterial)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to generate random invite key"))
	}

	// Hash it with SHA-3-256 for sufficient security
	inviteKey := sha3.Sum256(inviteKeyMaterial)

	inviteKeyStr := hex.EncodeToString(inviteKey[:])

	hierarchies, roleV2 := generateLegacyExtensionsFromLocationRoles(locationRoles)

	// Generate user certificate
	cert, privKey, err := certificate.GenerateX509UserCertificateAndKeyV2(
		certificate.CertificateRequestV2{
			Organization:  org,
			CommonName:    commonName,
			Role:          roleV2,
			Hierarchies:   hierarchies,
			LocationRoles: locationRoles,
		},
		caCert,
		caKey,
	)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to generate user certificate"))
	}

	// Encode certificate and private key
	certStr := encoder.EncodeX509Certificate(cert)
	derivedPassword := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(inviteKeyStr, userEmail)
	keyStr, err := encoder.EncodeEd25519PrivateKey(privKey, derivedPassword)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encode user private key"))
	}

	// Create return object
	result := make(map[string]interface{})
	result["certificate"] = certStr
	result["privateKey"] = keyStr
	result["derivedPassword"] = derivedPassword
	result["inviteKey"] = inviteKeyStr

	// JSON encode the result
	jsonResult, err := json.Marshal(result)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encode result"))
	}
	return string(jsonResult), nil
}

// GenerateLegacyExtensionsFromLocationRoles generates V1-style location hierarchies
// and certificate-level role from V2 LocationRoles for backward compatibility.
//
// This function bridges the gap between the new V2 LocationRoles system and the
// legacy V1 5-level hierarchy system used by UMH Classic instances. It transforms
// the flexible location-role mappings into the structured hierarchy format while
// determining an overall certificate-level role.
//
// The function analyzes all LocationRoles mappings to:
// 1. Generate corresponding 5-level LocationHierarchy structures
// 2. Determine the highest role across all locations for certificate-level role
// 3. Ensure at least one hierarchy exists (wildcard if none provided)
//
// Parameters:
//   - locationRoles: Map of location strings to roles from V2 certificate
//
// Returns:
//   - []certificate.LocationHierarchy: Array of V1-style 5-level hierarchies
//   - certificate.Role: Highest role found across all locations

// Location String Parsing:
//   - "*" or "" → Full wildcard hierarchy
//   - "umh" → Enterprise-level with wildcard sub-levels
//   - "umh.cologne.factory.line1" → Specific path with wildcard remainder
//   - Supports unlimited depth, truncated to 5 levels for V1 compatibility
//
// LIMITATIONS:
// If the location from the locationRoles contains more than 5 levels, it will be truncated to 5 levels.
//
// Example:
//
//	Input: "umh.cologne.factory.line1.station5.sensor3.module7" (7 levels)
//	Output: LocationHierarchy with Enterprise="umh", Site="cologne", Area="factory",
//	        ProductionLine="line1", WorkCell="station5" (levels "sensor3.module7" are ignored)
//
// When using this v2 certificate with UMH Classic, the user will have access to the location
// "umh.cologne.factory.line1.station5" and all sublocations of it.
func GenerateLegacyExtensionsFromLocationRoles(locationRoles certificate.LocationRoles) ([]certificate.LocationHierarchy, certificate.Role) {
	return generateLegacyExtensionsFromLocationRoles(locationRoles)
}

func generateLegacyExtensionsFromLocationRoles(locationRoles certificate.LocationRoles) ([]certificate.LocationHierarchy, certificate.Role) {
	var hierarchies []certificate.LocationHierarchy
	// here we generate the certificate-level role. We set it to the highest role from the locationRoles list
	roleV2 := certificate.RoleViewer
	// extract hierarchies from locationRoles
	seen := make(map[string]struct{}, len(locationRoles))
	for curLocation, curRole := range locationRoles {
		// determine highest role across all locations
		switch curRole {
		case certificate.RoleAdmin:
			roleV2 = certificate.RoleAdmin
		case certificate.RoleEditor:
			if roleV2 != certificate.RoleAdmin {
				roleV2 = certificate.RoleEditor
			}
		}

		loc := strings.TrimSpace(curLocation)
		if loc == "" || loc == "*" {
			// full wildcard hierarchy
			h := certificate.NewLocationHierarchy(
				certificate.NewWildcardLocation(certificate.LocationTypeEnterprise),
				certificate.NewWildcardLocation(certificate.LocationTypeSite),
				certificate.NewWildcardLocation(certificate.LocationTypeArea),
				certificate.NewWildcardLocation(certificate.LocationTypeProductionLine),
				certificate.NewWildcardLocation(certificate.LocationTypeWorkCell),
			)
			key := convertHierarchyToString(h)
			if _, ok := seen[key]; !ok {
				hierarchies = append(hierarchies, h)
				seen[key] = struct{}{}
			}
			continue
		}

		// Use safe parsing to validate and split location string
		segments, err := certificate.SafeParseLocationString(loc)
		if err != nil {
			// Skip invalid locations but continue processing others
			continue
		}
		parts := segments.Segments
		// build hierarchy from most specific provided part down, rest wildcard
		enterprise := certificate.NewWildcardLocation(certificate.LocationTypeEnterprise)
		site := certificate.NewWildcardLocation(certificate.LocationTypeSite)
		area := certificate.NewWildcardLocation(certificate.LocationTypeArea)
		prodLine := certificate.NewWildcardLocation(certificate.LocationTypeProductionLine)
		workCell := certificate.NewWildcardLocation(certificate.LocationTypeWorkCell)

		if len(parts) >= 1 && parts[0] != "*" {
			enterprise = certificate.NewLocation(certificate.LocationTypeEnterprise, parts[0])
		}
		if len(parts) >= 2 && parts[1] != "*" {
			site = certificate.NewLocation(certificate.LocationTypeSite, parts[1])
		}
		if len(parts) >= 3 && parts[2] != "*" {
			area = certificate.NewLocation(certificate.LocationTypeArea, parts[2])
		}
		if len(parts) >= 4 && parts[3] != "*" {
			prodLine = certificate.NewLocation(certificate.LocationTypeProductionLine, parts[3])
		}
		if len(parts) >= 5 && parts[4] != "*" {
			workCell = certificate.NewLocation(certificate.LocationTypeWorkCell, parts[4])
		}

		h := certificate.NewLocationHierarchy(enterprise, site, area, prodLine, workCell)
		key := convertHierarchyToString(h)
		if _, ok := seen[key]; !ok {
			hierarchies = append(hierarchies, h)
			seen[key] = struct{}{}
		}
	}

	if len(hierarchies) == 0 {
		// ensure at least one hierarchy exists
		hierarchies = append(hierarchies, certificate.NewLocationHierarchy(
			certificate.NewWildcardLocation(certificate.LocationTypeEnterprise),
			certificate.NewWildcardLocation(certificate.LocationTypeSite),
			certificate.NewWildcardLocation(certificate.LocationTypeArea),
			certificate.NewWildcardLocation(certificate.LocationTypeProductionLine),
			certificate.NewWildcardLocation(certificate.LocationTypeWorkCell),
		))
	}

	return hierarchies, roleV2
}

// GenerateInstanceCertificateAndEncode generates a server certificate for UMH instances
// with DNS names and IP addresses for TLS server authentication.
//
// This function creates X.509 server certificates for UMH instances that need to
// provide TLS endpoints. The certificates include Subject Alternative Names (SAN)
// with DNS names and IP addresses for flexible server identification.
//
// Parameters:
//   - org: Organization name for the certificate
//   - commonName: Common name (typically the primary hostname) for the certificate
//   - dnsNames: Array of DNS names to include in SAN extension
//   - ipAddresses: Array of IP addresses to include in SAN extension
//   - caCertStr: PEM-encoded CA certificate string
//   - caKeyStr: Encrypted PEM-encoded CA private key string
//   - caDerivedPassword: CA private key decryption password
//   - instancePassword: Instance's password for private key encryption
//   - instanceUUID: Instance's UUID for password derivation
//
// Returns:
//   - string: JSON-encoded certificate bundle (certificate, privateKey, derivedPassword)
//   - error: Any error that occurred during generation
//

func GenerateInstanceCertificateAndEncode(org string, commonName string, dnsNames []string, ipAddresses []net.IP, caCertStr string, caKeyStr string, caDerivedPassword string, instancePassword string, instanceUUID string) (string, error) {

	// Decode CA certificate and private key
	caCert, err := encoder.DecodeX509Certificate(caCertStr)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to decode CA certificate"))
	}

	caKey, err := encoder.DecodeEd25519PrivateKey(caKeyStr, caDerivedPassword)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to decode CA private key"))
	}
	// Generate instance certificate
	cert, privKey, err := certificate.GenerateX509InstanceCertificateAndKey(
		certificate.InstanceRequest{
			Organization: org,
			CommonName:   commonName,
			DNSNames:     dnsNames,
			IPAddresses:  ipAddresses,
		},
		caCert,
		caKey,
	)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to generate instance certificate"))
	}

	// Encode certificate and private key
	certStr := encoder.EncodeX509Certificate(cert)
	derivedPassword := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(instancePassword, instanceUUID)
	keyStr, err := encoder.EncodeEd25519PrivateKey(privKey, derivedPassword)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encode instance private key"))
	}
	// Create return object
	result := make(map[string]interface{})
	result["certificate"] = certStr
	result["privateKey"] = keyStr
	result["derivedPassword"] = derivedPassword
	// JSON encode the result
	jsonResult, err := json.Marshal(result)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encode result"))
	}
	return string(jsonResult), nil
}

// ValidateCACertificateAndPrivateKey validates that a CA certificate and private key
// are properly formatted and can be decoded with the provided password.
//
// This function performs basic validation to ensure that the CA certificate and
// private key are valid and can be used for certificate signing operations.
// It checks both the certificate format and private key decryption.
//
// Parameters:
//   - caCertStr: PEM-encoded CA certificate string
//   - caKeyStr: Encrypted PEM-encoded CA private key string
//   - caDerivedPassword: Password for decrypting the CA private key
//
// Returns:
//   - bool: true if both certificate and private key are valid, false otherwise
//   - error: Any error that occurred during validation
//
// Validation Checks:
//   - CA certificate can be decoded from PEM format
//   - CA private key can be decrypted with the provided password
//   - No cryptographic validation of key pair matching is performed
//
// Note: This function only validates format and decryption, not the mathematical
// relationship between the certificate and private key.
func ValidateCACertificateAndPrivateKey(caCertStr string, caKeyStr string, caDerivedPassword string) (bool, error) {

	_, err := encoder.DecodeEd25519PrivateKey(caKeyStr, caDerivedPassword)
	if err != nil {
		return false, errors.Join(err, errors.New("failed to decode CA private key"))
	}
	// Validate the CA certificate
	_, err = encoder.DecodeX509Certificate(caCertStr)
	if err != nil {
		return false, errors.Join(err, errors.New("failed to decode CA certificate"))
	}
	return true, nil
}

// ValidateUserCertificateAndPrivateKey validates a user certificate against a CA
// certificate and verifies the private key can be decrypted.
//
// This function performs comprehensive validation of a user certificate including
// certificate chain verification against the CA and private key decryption.
// It ensures the certificate is properly signed and the private key is accessible.
//
// Parameters:
//   - userCertStr: PEM-encoded user certificate string
//   - userKeyStr: Encrypted PEM-encoded user private key string
//   - caCertStr: PEM-encoded CA certificate string for chain validation
//   - userDerivedPassword: Password for decrypting the user private key
//
// Returns:
//   - bool: true if certificate is valid and private key can be decrypted
//   - error: Any error that occurred during validation
//
// Validation Steps:
//  1. Decode CA certificate and create certificate pool
//  2. Decode user certificate
//  3. Verify certificate chain (user cert signed by CA)
//  4. Verify certificate is valid for client authentication
//  5. Decrypt and validate private key
//
// Security Checks:
//   - Certificate signature verification against CA
//   - Certificate validity period (not before/after)
//   - Key usage validation for client authentication
//   - Private key format and encryption validation
func ValidateUserCertificateAndPrivateKey(userCertStr string, userKeyStr string, caCertStr string, userDerivedPassword string) (bool, error) {
	// Decode the CA certificate
	caCert, err := encoder.DecodeX509Certificate(caCertStr)
	if err != nil {
		return false, errors.Join(err, errors.New("failed to decode CA certificate"))
	}
	// Decode the user certificate
	userCert, err := encoder.DecodeX509Certificate(userCertStr)
	if err != nil {
		return false, errors.Join(err, errors.New("failed to decode user certificate"))
	}
	// Create a cert pool and add the CA cert
	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	// Verify the certificate
	opts := x509.VerifyOptions{
		Roots: roots,
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
		},
	}
	_, err = userCert.Verify(opts)
	if err != nil {
		return false, errors.Join(err, errors.New("certificate verification failed"))
	}

	// Derive password and attempt to decode private key
	_, err = encoder.DecodeEd25519PrivateKey(userKeyStr, userDerivedPassword)
	if err != nil {
		return false, errors.Join(err, errors.New("failed to decode user private key"))
	}

	return true, nil
}

// ValidateUserCertificateAndPrivateKeyV2 validates a user certificate against
// multiple CA certificates and verifies the private key can be decrypted.
//
// This V2 validation function supports certificate validation against multiple
// CA certificates, enabling certificate chain validation in scenarios where
// certificates may be signed by different CAs (e.g., CA rotation, multiple
// trust anchors).
//
// Parameters:
//   - userCertStr: PEM-encoded user certificate string
//   - userKeyStr: Encrypted PEM-encoded user private key string
//   - caCertStr: Array of PEM-encoded CA certificate strings
//   - userDerivedPassword: Password for decrypting the user private key
//
// Returns:
//   - bool: true if certificate is valid against any CA and private key can be decrypted
//   - error: Any error that occurred during validation
//
// Enhanced Features:
//   - Multiple CA certificate support for trust chain flexibility
//   - Supports CA rotation scenarios
//   - Validates against any valid CA in the provided list
//   - Same security validation as V1 function
//
// Use Cases:
//   - CA certificate rotation and migration
//   - Multi-tenant environments with different CAs
//   - Federated authentication scenarios
//   - Cross-organization certificate validation
func ValidateUserCertificateAndPrivateKeyV2(userCertStr string, userKeyStr string, caCertStr []string, userDerivedPassword string) (bool, error) {
	// Decode the user certificate
	userCert, err := encoder.DecodeX509Certificate(userCertStr)
	if err != nil {
		return false, errors.Join(err, errors.New("failed to decode user certificate"))
	}

	// Create a cert pool and add all CA certs
	roots := x509.NewCertPool()
	for i, caCertString := range caCertStr {
		caCert, err := encoder.DecodeX509Certificate(caCertString)
		if err != nil {
			return false, errors.Join(err, fmt.Errorf("failed to decode CA certificate at index %d", i))
		}
		roots.AddCert(caCert)
	}

	// Verify the certificate
	opts := x509.VerifyOptions{
		Roots: roots,
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
		},
	}
	_, err = userCert.Verify(opts)
	if err != nil {
		return false, errors.Join(err, errors.New("certificate verification failed"))
	}

	// Derive password and attempt to decode private key
	_, err = encoder.DecodeEd25519PrivateKey(userKeyStr, userDerivedPassword)
	if err != nil {
		return false, errors.Join(err, errors.New("failed to decode user private key"))
	}

	return true, nil
}

// ValidateInstanceCertificateAndPrivateKey validates an instance certificate
// against a CA certificate and verifies the private key can be decrypted.
//
// This function performs validation specifically for server/instance certificates
// that are used for TLS server authentication. It validates the certificate
// chain and ensures the private key is accessible for TLS operations.
//
// Parameters:
//   - instanceCertStr: PEM-encoded instance certificate string
//   - instanceKeyStr: Encrypted PEM-encoded instance private key string
//   - caCertStr: PEM-encoded CA certificate string for chain validation
//   - instanceDerivedPassword: Password for decrypting the instance private key
//
// Returns:
//   - bool: true if certificate is valid and private key can be decrypted
//   - error: Any error that occurred during validation
//
// Validation Features:
//   - Certificate chain verification against CA
//   - Server authentication key usage validation (ExtKeyUsageServerAuth)
//   - Private key decryption and format validation
//   - Instance-specific password derivation support
//
// Use Cases:
//   - TLS server certificate validation
//   - Instance deployment verification
//   - Server certificate renewal validation
//   - Load balancer certificate setup
func ValidateInstanceCertificateAndPrivateKey(instanceCertStr string, instanceKeyStr string, caCertStr string, instanceDerivedPassword string) (bool, error) {
	// Decode the CA certificate
	caCert, err := encoder.DecodeX509Certificate(caCertStr)
	if err != nil {
		return false, errors.Join(err, errors.New("failed to decode CA certificate"))
	}
	// Decode the instance certificate
	instanceCert, err := encoder.DecodeX509Certificate(instanceCertStr)
	if err != nil {
		return false, errors.Join(err, errors.New("failed to decode instance certificate"))
	}
	// Create a cert pool and add the CA cert
	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	// Verify the certificate
	opts := x509.VerifyOptions{
		Roots: roots,
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}
	_, err = instanceCert.Verify(opts)
	if err != nil {
		return false, errors.Join(err, errors.New("certificate verification failed"))
	}
	// Derive password and attempt to decode private key
	_, err = encoder.DecodeEd25519PrivateKey(instanceKeyStr, instanceDerivedPassword)
	if err != nil {
		return false, errors.Join(err, errors.New("failed to decode instance private key"))
	}
	return true, nil
}

// ExtractCertificateRoleAndHierarchies extracts the role and location hierarchies
// from a certificate and returns them in a JSON format.
//
// This function provides compatibility support for extracting legacy V1 certificate
// information including the certificate-level role and 5-level location hierarchies.
// It's primarily used for displaying certificate information and migration tools.
//
// Parameters:
//   - certStr: PEM-encoded X.509 certificate string
//
// Returns:
//   - string: JSON object containing role and hierarchies information
//   - error: Any error that occurred during extraction or encoding
//
// JSON Format:
//
//	{
//	  "role": "Admin|Editor|Viewer|",
//	  "hierarchies": [
//	    {
//	      "enterprise": {"type": "enterprise", "value": "umh"},
//	      "site": {"type": "site", "value": "cologne"},
//	      "area": {"type": "area", "value": "factory"},
//	      "productionLine": {"type": "productionLine", "value": "line1"},
//	      "workCell": {"type": "workCell", "value": "*"}
//	    }
//	  ]
//	}
//
// Backward Compatibility:
//   - Works with both V1 and V2 certificates
//   - V2 certificates extract legacy extensions for compatibility
//   - Returns empty values if no role/hierarchies are found
//   - Maintains structure expected by legacy frontend components
func ExtractCertificateRoleAndHierarchies(certStr string) (string, error) {
	// Decode the certificate
	cert, err := encoder.DecodeX509Certificate(certStr)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to decode certificate"))
	}

	// Extract role
	role, err := certificate.GetRoleFromCertificate(cert)
	if err != nil {
		// If no role is found, set it to empty string
		role = ""
	}

	// Extract hierarchies
	hierarchies, err := certificate.GetLocationHierarchiesFromCertificate(cert)
	if err != nil {
		// If no hierarchies are found, set to empty array
		hierarchies = []certificate.LocationHierarchy{}
	}

	// Convert hierarchies to a serializable format
	hierarchiesData := make([]map[string]interface{}, 0, len(hierarchies))
	for _, h := range hierarchies {
		hierarchyData := map[string]interface{}{
			"enterprise":     locationToMap(h.Enterprise),
			"site":           locationToMap(h.Site),
			"area":           locationToMap(h.Area),
			"productionLine": locationToMap(h.ProductionLine),
			"workCell":       locationToMap(h.WorkCell),
		}
		hierarchiesData = append(hierarchiesData, hierarchyData)
	}

	// Create return object
	result := make(map[string]interface{})
	result["role"] = role
	result["hierarchies"] = hierarchiesData

	// JSON encode the result
	jsonResult, err := json.Marshal(result)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encode result"))
	}
	return string(jsonResult), nil
}

// locationToMap converts a Location structure to a map for JSON serialization.
//
// This helper function transforms a Location object into a map format suitable
// for JSON encoding, making it easy to serialize location information for
// API responses and frontend consumption.
//
// Parameters:
//   - loc: Location object containing type and value information
//
// Returns:
//   - map[string]string: Map with "type" and "value" keys
//
// Example Output:
//
//	{"type": "enterprise", "value": "umh"}
//	{"type": "site", "value": "*"}
func locationToMap(loc certificate.Location) map[string]string {
	return map[string]string{
		"type":  string(loc.Type),
		"value": loc.Value,
	}
}

// ReEncodePrivateKey changes the encryption password of an encrypted private key.
//
// This function enables secure password changes by decrypting a private key with
// the old password and re-encrypting it with a new password. It uses password
// derivation to convert user passwords into encryption keys.
//
// Parameters:
//   - encoded: Currently encrypted PEM-encoded private key string
//   - oldPassword: Current password for decrypting the private key
//   - newPassword: New password for re-encrypting the private key
//   - email: User's email address for password derivation
//
// Returns:
//   - string: Re-encrypted private key with new password
//   - error: Any error that occurred during re-encryption
//
// Security Features:
//   - Uses password derivation for both old and new passwords
//   - Secure key decryption/re-encryption process
//   - No intermediate plaintext key storage
//   - Maintains Ed25519 key format and strength
//
// Use Cases:
//   - User password changes
//   - Security policy compliance (password rotation)
//   - Key migration between systems
//   - Recovery from password compromise
func ReEncodePrivateKey(encoded string, oldPassword string, newPassword string, email string) (string, error) {
	oldDerivedPassword := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(oldPassword, email)
	newDerivedPassword := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(newPassword, email)
	return encoder.ReEncodeEd25519PrivateKey(encoded, oldDerivedPassword, newDerivedPassword)
}

// EncryptPrivateKeyWithPublicKey encrypts a private key using public key cryptography
// for secure key sharing between users.
//
// This function implements secure key sharing by using the recipient's public key
// (extracted from their certificate) to encrypt a private key. This enables secure
// transfer of private keys between users without requiring shared passwords.
//
// Parameters:
//   - ourPrivateKey: Our encrypted private key string for signing/encryption
//   - ourPrivateKeyPassword: Password to decrypt our private key
//   - ourEmail: Our email address for password derivation
//   - theirX509Certificate: Recipient's PEM-encoded certificate containing their public key
//   - keyToEncrypt: The private key to encrypt for sharing
//   - tempEncryptionPassword: Current encryption password of the key to encrypt
//   - userEmail: Email address associated with the key to encrypt
//
// Returns:
//   - string: Public key encrypted private key suitable for secure transmission
//   - error: Any error that occurred during encryption
//
// Use Cases:
//   - Admin delegation of user certificates
//   - Secure key recovery processes
//   - Key sharing between authorized users
//   - Cross-user certificate management
func EncryptPrivateKeyWithPublicKey(
	ourPrivateKey string,
	ourPrivateKeyPassword string,
	ourEmail string,
	theirX509Certificate string,
	keyToEncrypt string,
	tempEncryptionPassword string,
	userEmail string,
) (string, error) {
	// Step 1: Decrypt our private key
	decryptedPrivateKey, err := encoder.DecodeEd25519PrivateKey(ourPrivateKey, ourPrivateKeyPassword)
	if err != nil {
		// Derive caKeyPassword from userPassword
		caKeyPassword := password_derivation.DeriveCAEncryptionKeyFromPassword(ourPrivateKeyPassword, ourEmail)
		// Try again
		decryptedPrivateKey, err = encoder.DecodeEd25519PrivateKey(ourPrivateKey, caKeyPassword)
		if err != nil {
			return "", errors.Join(err, errors.New("failed to decode CA private key"))
		}
	}

	// Step 2: Decrypt the key to encrypt
	decryptedKeyToEncrypt, err := encoder.DecodeEd25519PrivateKey(keyToEncrypt, tempEncryptionPassword)
	if err != nil {
		tempDerivedPassword := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(tempEncryptionPassword, userEmail)
		decryptedKeyToEncrypt, err = encoder.DecodeEd25519PrivateKey(keyToEncrypt, tempDerivedPassword)
		if err != nil {
			return "", errors.Join(err, errors.New("failed to decrypt the key to encrypt"))
		}
	}

	// Step 3: Decode their X509 certificate
	theirCertificate, err := encoder.DecodeX509Certificate(theirX509Certificate)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to decode their X509 certificate"))
	}

	// Step 4: Extract their public key from the certificate
	theirPublicKey, err := certificate.GetPublicKeyFromCertificate(theirCertificate)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to extract their public key from the certificate"))
	}

	// Step 5: Encrypt the key to encrypt with the public key
	encryptedKey, err := encoder.EncodeEd25519PrivateKeyWithPublicKey(decryptedPrivateKey, theirPublicKey, decryptedKeyToEncrypt)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to encrypt the key to encrypt"))
	}

	return encryptedKey, nil
}

// DecryptPrivateKeyWithPrivateKey performs asymmetric decryption of an encrypted private key
// using ECDH (Elliptic Curve Diffie-Hellman) key exchange between two key pairs.
//
// This function enables secure private key transmission in certificate patch scenarios where
// an administrator needs to provide a user with a new encrypted private key. The admin encrypts
// the user's new private key using the user's public key (from their certificate), and the user
// can decrypt it using their own private key and the admin's public key.
//
// Cryptographic Process:
//  1. Derives shared secret using ECDH between user's private key and admin's public key
//  2. Uses the shared secret to decrypt the encrypted private key
//  3. Re-encrypts the decrypted private key with the user's password for storage
//
// Parameters:
//   - ourPublicKeyEncryptedPrivateKey: New private key encrypted by admin using user's public key
//   - ourPrivateKey: User's current encrypted private key (for ECDH key exchange)
//   - ourEmail: User's email address (used for password derivation if needed)
//   - theirX509Certificate: Admin's PEM certificate containing the public key used for encryption
//   - ourPrivateKeyPassword: User's derived password for decrypting/re-encrypting private keys
//
// Returns:
//   - string: User's new private key, decrypted and re-encrypted with their password
//   - error: Decryption, certificate parsing, or re-encryption errors
//
// Security Context:
//   - Enables secure certificate patches without transmitting plaintext private keys
//   - Maintains end-to-end encryption throughout the key update process
//   - Requires both parties' key material to complete the decryption
func DecryptPrivateKeyWithPrivateKey(
	ourPublicKeyEncryptedPrivateKey string,
	ourPrivateKey string,
	ourEmail string,
	theirX509Certificate string,
	ourPrivateKeyPassword string,
) (string, error) {

	// Step 1: Decrypt our private key
	decryptedPrivateKey, err := encoder.DecodeEd25519PrivateKey(ourPrivateKey, ourPrivateKeyPassword)
	if err != nil {

		// Derive caKeyPassword from userPassword
		ourPrivateKeyPassword = password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(ourPrivateKeyPassword, ourEmail)
		// Try again
		decryptedPrivateKey, err = encoder.DecodeEd25519PrivateKey(ourPrivateKey, ourPrivateKeyPassword)
		if err != nil {

			return "", errors.Join(err, errors.New("failed to decrypt our private key"))
		}
	}

	// Step 2: Decode their X509 certificate
	theirCertificate, err := encoder.DecodeX509Certificate(theirX509Certificate)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to decode their X509 certificate"))
	}

	// Step 3: Extract their public key from the certificate
	theirPublicKey, err := certificate.GetPublicKeyFromCertificate(theirCertificate)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to extract their public key from the certificate"))
	}

	// Step 4: Decrypt the encrypted private key using our private key and their public key
	decryptedPrivateKeyBytes, err := encoder.DecodeEd25519PrivateKeyWithPrivateKey(
		ourPublicKeyEncryptedPrivateKey,
		decryptedPrivateKey,
		ourPrivateKeyPassword,
		theirPublicKey,
	)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to decrypt the encrypted private key"))
	}

	// Step 5: Re-encrypt the decrypted private key with the new password
	reEncryptedPrivateKey, err := encoder.EncodeEd25519PrivateKey(decryptedPrivateKeyBytes, ourPrivateKeyPassword)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to re-encrypt the private key"))
	}

	return reEncryptedPrivateKey, nil
}

// ValidateKeyDecryption validates that an encrypted private key can be decrypted
// with the provided password and email combination.
//
// This function performs a simple validation check to ensure that a private key
// can be successfully decrypted using the provided credentials. It's useful for
// password verification and key integrity checks.
//
// Parameters:
//   - encryptedPrivateKey: PEM-encoded encrypted private key string
//   - password: User's password for key decryption
//   - email: User's email address for password derivation
//
// Returns:
//   - bool: true if the key can be decrypted successfully, false otherwise
//   - error: Any error that occurred during decryption attempt
//
// Use Cases:
//   - Password verification before key operations
//   - Key integrity validation
//   - User authentication workflows
//   - Pre-flight checks for key operations
func ValidateKeyDecryption(encryptedPrivateKey string, password string, email string) (bool, error) {
	derivedKey := password_derivation.DerivePrivateKeyEncryptionKeyFromPassword(password, email)

	_, err := encoder.DecodeEd25519PrivateKey(encryptedPrivateKey, derivedKey)
	if err != nil {
		return false, errors.Join(err, errors.New("failed to decrypt private key"))
	}

	return true, nil
}

// ValidateKeyDecryptionUsingDerivedPassword validates that an encrypted private key
// can be decrypted using a pre-derived password.
//
// This function is similar to ValidateKeyDecryption but uses an already derived
// password instead of deriving it from a user password and email. This is useful
// when working with systems that pre-compute derived passwords for performance
// or when the original password/email combination is not available.
//
// Parameters:
//   - encryptedPrivateKey: PEM-encoded encrypted private key string
//   - derivedPassword: Pre-derived password for key decryption
//
// Returns:
//   - bool: true if the key can be decrypted successfully, false otherwise
//   - error: Any error that occurred during decryption attempt

// Use Cases:
//   - Systems with cached derived passwords
//   - Performance-optimized validation workflows
//   - Integration with external password derivation systems
//   - Validation with invite keys or temporary passwords
func ValidateKeyDecryptionUsingDerivedPassword(encryptedPrivateKey string, derivedPassword string) (bool, error) {

	_, err := encoder.DecodeEd25519PrivateKey(encryptedPrivateKey, derivedPassword)
	if err != nil {
		return false, errors.Join(err, errors.New("failed to decrypt private key"))
	}
	return true, nil
}
