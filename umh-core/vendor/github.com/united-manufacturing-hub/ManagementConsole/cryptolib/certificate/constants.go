// Package constants defines all system-wide limitations, cryptographic parameters,
// and configuration values for the V2 certificate system.
//
// This file centralizes all hardcoded values to make the system easier to understand,
// maintain, and potentially configure in the future. Each constant includes detailed
// documentation explaining the reasoning behind the chosen value and its implications.
package certificate

import (
	"fmt"
	"time"
)

// # Certificate Chain and Path Limitations

// MaxCertificatePathLength defines the maximum depth of certificate chains.
//
// **Value**: 10 levels
//
// **Reasoning**: This limit prevents excessively long certificate chains that could:
// - Impact validation performance (each level requires cryptographic verification)
// - Increase attack surface (more certificates = more potential compromise points)
// - Complicate troubleshooting and certificate management
// - Enable denial-of-service attacks through deeply nested chains
//
// **Implications**:
// - CA hierarchy depth: Maximum 10 intermediate CAs between root and end certificates
// - Admin delegation depth: Maximum 10 levels of admin-to-admin delegation in V2 system
// - Certificate validation time increases linearly with chain depth
//
// **Industry Context**: Most PKI systems use similar limits (typically 5-20 levels).
// Common values: Browser CAs (10), Enterprise PKI (5-15), Internal systems (10-20).
const MaxCertificatePathLength = 10

// # Certificate Validity Periods

// CertificateValidityYears defines how long all certificates remain valid.
//
// **Value**: 100 years
//
// **Reasoning**: Long validity periods chosen to:
// - Minimize operational overhead of certificate renewal
// - Reduce system downtime from expired certificates
// - Simplify deployment in air-gapped or difficult-to-update environments
// - Match the expected operational lifetime of industrial systems (decades)
//
// **Implications**:
// - Revoked certificates must be tracked for the entire 100-year period
// - Requires robust Certificate Revocation List (CRL) or OCSP infrastructure
// - Compromised certificates remain cryptographically valid until revocation is checked
// - Clock skew tolerance built in with 5-minute backdating of NotBefore
//
// **Security Trade-off**: Convenience vs. revocation complexity. In industrial IoT
// environments, the operational benefits typically outweigh the revocation burden.
const CertificateValidityYears = 100

// CertificateClockSkewTolerance defines how far back in time the NotBefore field is set.
//
// **Value**: 5 minutes
//
// **Reasoning**: Accounts for clock synchronization issues between:
// - Certificate generation system and validation systems
// - Distributed systems with imperfect time synchronization
// - Network delays in certificate distribution
//
// **Implications**: Certificates become valid 5 minutes before their actual generation time.
const CertificateClockSkewTolerance = 5 * time.Minute

// # Cryptographic Algorithm Specifications

// CertificateKeyAlgorithm documents the mandatory key algorithm for all certificates.
//
// **Value**: Ed25519 (Edwards-curve Digital Signature Algorithm)
//
// **Reasoning**: Ed25519 chosen for:
// - Security: Immune to timing attacks, constant-time operations
// - Performance: Faster signature generation and verification than RSA
// - Key size: 32-byte keys vs 2048+ bit RSA keys (256+ bytes)
// - Simplicity: No parameter choices, reduces implementation errors
// - Modern cryptography: Designed to avoid historical mistakes in DSA/ECDSA
//
// **Implications**:
// - All certificates in the system use identical key algorithms
// - Simplified implementation with single key type support
// - Interoperability limited to Ed25519-capable systems
// - Cannot use RSA, DSA, or other ECDSA curves
//
// **Note**: This is a documentation constant only. The actual algorithm
// is enforced through code implementation, not configurable.
const CertificateKeyAlgorithm = "Ed25519"

// Ed25519KeySizeBytes defines the key size for Ed25519 keys.
//
// **Value**: 32 bytes (256 bits)
//
// **Security Level**: Equivalent to ~3072-bit RSA or 256-bit AES
//
// **Note**: This is fixed by the Ed25519 specification and cannot be changed.
const Ed25519KeySizeBytes = 32

// Ed25519SignatureSizeBytes defines the signature size for Ed25519 signatures.
//
// **Value**: 64 bytes (512 bits)
//
// **Note**: This is fixed by the Ed25519 specification and cannot be changed.
const Ed25519SignatureSizeBytes = 64

// # Role and Permission System Limitations

// SupportedRoles documents the complete set of available roles in the system.
//
// **Available Roles**:
// - Admin: Full access, can delegate permissions, CA capabilities
// - Editor: Can modify data and configurations
// - Viewer: Read-only access
//
// **Reasoning**: Limited role set for:
// - Simplicity in permission logic and UI
// - Clear security boundaries
// - Easier troubleshooting and audit
//
// **Implications**:
// - No custom roles or fine-grained permissions beyond location-based access
// - Permission granularity achieved through location path specificity
// - Role escalation only possible through certificate re-issuance
var SupportedRoles = []Role{RoleAdmin, RoleEditor, RoleViewer}

// parseRole validates role values for use throughout the certificate system.
// This function provides centralized role validation to ensure consistency across all
// certificate generation and validation operations.
//
// **Validation Rules**:
// - Empty/blank roles return an error (no default role assigned)
// - Only RoleAdmin, RoleEditor, and RoleViewer are valid
// - Case-sensitive matching (must match exact constant values)
// - Any other value returns an error
//
// **Security**: No default role is assigned to prevent ambiguous permission scenarios.
// Calling code must explicitly specify the intended role.
//
// Parameters:
//   - role: Role value to validate
//
// Returns:
//   - Role: Validated role
//   - error: Error if role is invalid (empty, blank, or not a valid role constant)
func parseRole(role Role) (Role, error) {
	// Handle empty/blank role - return error instead of defaulting
	if role == "" {
		return "", fmt.Errorf("role cannot be empty: must be one of %v", SupportedRoles)
	}

	// Validate against supported roles
	for _, supportedRole := range SupportedRoles {
		if role == supportedRole {
			return role, nil
		}
	}

	// Invalid role provided
	return "", fmt.Errorf("invalid role '%s': must be one of %v", role, SupportedRoles)
}

// # Location Path Format Specifications

// LocationPathSeparator defines the character used to separate location hierarchy levels.
//
// **Value**: "." (dot)
//
// **Reasoning**: Dot notation chosen because we also use it in UMH as "location_path".
//
// **Examples**: "umh.cologne.factory.line1.station5"
const LocationPathSeparator = "."

// LocationWildcard defines the wildcard character for location matching.
//
// **Value**: "*" (asterisk)
//
// **Reasoning**: Standard wildcard character, universally understood
//
// **Usage**: "*" grants access to all locations, used in CA certificates and global admin roles
const LocationWildcard = "*"

// # Password and Key Derivation Parameters

// InviteKeyMaterialBytes defines the size of random material for invite key generation.
//
// **Value**: 4092 bytes
//
// **Reasoning**: Large random input for:
// - High entropy before SHA-3-256 hashing
// - Protection against potential hash function weaknesses
// - Comfortable security margin
//
// **Note**: Final invite key is 32 bytes after SHA-3-256 hashing
const InviteKeyMaterialBytes = 4092

// InviteKeyHashOutputBytes defines the final invite key size after hashing.
//
// **Value**: 32 bytes (256 bits)
//
// **Reasoning**: SHA-3-256 output size, provides 128-bit security level
const InviteKeyHashOutputBytes = 32

// # X.509 Extension Object Identifiers (OIDs)

// UMHPrivateEnterpriseNumber is the IANA-assigned Private Enterprise Number for UMH.
//
// **Value**: 59193
//
// **Usage**: Base for all UMH-specific X.509 certificate extensions
// **Format**: 1.3.6.1.4.1.59193.x.y (where x.y are UMH-defined extension types)
const UMHPrivateEnterpriseNumber = 59193

// # Certificate Extension Version Numbers

// ExtensionVersionV1 identifies V1 certificate extensions (legacy).
//
// **Extensions**:
// - 1.3.6.1.4.1.59193.1.1: Role extension
// - 1.3.6.1.4.1.59193.1.2: Location hierarchy extension
const ExtensionVersionV1 = 1

// ExtensionVersionV2 identifies V2 certificate extensions (current).
//
// **Extensions**:
// - 1.3.6.1.4.1.59193.2.1: LocationRoles extension
const ExtensionVersionV2 = 2

// # Security Enforcement Policies

// V2SecurityEnforcementPolicy documents the mixed-chain validation behavior.
//
// **Policy**: If ANY certificate in a chain (root CA, intermediates, or user cert)
// contains V2 LocationRole extension, then ALL certificates in that chain must
// follow V2 validation rules.
//
// **Reasoning**: Prevents downgrade attacks where:
// - Attacker uses V1 user certificate (no LocationRole validation)
// - In a V2-capable certificate chain (has LocationRole-aware CAs)
// - Bypassing the granular location-based permission system
//
// **Implementation**: ValidateX509UserCertificate enforces this policy
//
// **Validation Matrix**:
// | Chain Type | User Cert | Result |
// |------------|-----------|---------|
// | V1 only    | V1        | ✅ Pass (backward compatibility) |
// | V1 only    | V2        | ✅ Pass (forward compatibility) |
// | V2 mixed   | V1        | ❌ FAIL (security enforcement) |
// | V2 mixed   | V2        | ✅ Pass (full V2 validation) |

// # System Architecture Constraints

// MaxLocationDepth documents that V2 system supports unlimited location hierarchy depth.
//
// **V1 Limitation**: 5 levels (Enterprise→Site→Area→ProductionLine→WorkCell)
// **V2 Capability**: Unlimited depth using dot-separated paths
//
// **Note**: While unlimited, practical limits exist due to:
// - Certificate size (longer paths = larger certificates)
// - Performance impact of hierarchical permission resolution
// - Human comprehension and management complexity
//
// **Recommendation**: Keep hierarchy depth reasonable (typically < 20 levels)
const MaxLocationDepth = -1 // -1 indicates unlimited

// SerialNumberBitLength defines the bit length of certificate serial numbers.
//
// **Value**: 128 bits
//
// **Reasoning**: RFC 5280 recommendation for unique serial numbers
// **Collision Probability**: Negligible for practical certificate volumes
const SerialNumberBitLength = 128

// # Performance and Resource Limits

// CertificateExtensionSizeLimit defines the maximum size in bytes for a single certificate extension.
//
// **Value**: 16384 bytes (16 KB)
//
// **Reasoning**: This limit provides a safe balance between functionality and security:
// - Allows large LocationRoles mappings (hundreds of locations per certificate)
// - Prevents DoS attacks through extremely large extension data
// - Compatible with most TLS libraries and protocol implementations
// - Leaves room for future extension growth while maintaining reasonable certificate sizes
//
// **Typical extension sizes**:
// - Role extension: ~20 bytes
// - Location extension (5 hierarchies): ~500 bytes
// - LocationRoles extension (100 locations): ~5KB
// - LocationRoles extension (500 locations): ~25KB (would exceed limit)
//
// **Security**: Extensions exceeding this limit are rejected during certificate
// construction and parsing to prevent memory exhaustion and DoS attacks.
//
// **Configuration**: This limit can be adjusted by changing this constant and
// recompiling the system. Consider impact on certificate processing libraries.
const CertificateExtensionSizeLimit = 16384

// # Version-Aware Authentication System in the backend
//
// The certificate system supports three distinct compatibility modes to ensure
// backwards compatibility while enabling enhanced security features in newer deployments.
// Each mode is automatically detected based on the company's certificate configuration.

// ## Authentication Version Detection Matrix
//
// **Company Certificate State**:
// | Company Cert | LocationRoles Ext | Detected Version | Validation Method |
// |--------------|-------------------|------------------|-------------------|
// | None         | N/A               | v0 (Legacy)      | Owner-based only  |
// | Present      | Absent            | v1 (Basic)       | Direct CA validation |
// | Present      | Present           | v2 (Advanced)    | Full chain validation |

// ## Version 0 (v0): Legacy Authentication
//
// **Detection**: Company has no certificate (company.Certificate == nil)
//
// **Authentication Logic**:
// - Users without certificates: Granted admin access (legacy compatibility)
// - Users with certificates: Rejected (inconsistent state)
// - Company owners: Always granted admin access (backwards compatibility)
//
// **Use Cases**:
// - Existing deployments before certificate system introduction
// - Companies that haven't migrated to certificate-based authentication
// - Development/testing environments without PKI setup
//
// **Security Model**: Relies entirely on database-based company ownership
//
// **Migration Path**: Companies can upgrade by generating CA certificates

// ## Version 1 (v1): Basic Certificate Authentication
//
// **Detection**: Company has certificate without LocationRoles extension
//
// **Authentication Logic**:
// - User certificates validated directly against company CA certificate
// - No intermediate certificates supported
// - Role extracted from V1 Role extension (1.3.6.1.4.1.59193.1.1)
// - Location permissions via V1 Location extension (1.3.6.1.4.1.59193.1.2)
//
// **Certificate Chain**: User Cert → Company CA (max 2 levels)
//
// **Extensions Used**:
// - Role extension for user permissions
// - Location extension for hierarchical access control
//
// **Security Model**: Direct trust relationship, simpler validation
//
// **Migration Path**: Can upgrade to v2 by generating LocationRoles-capable CA

// ## Version 2 (v2): Advanced Chain Authentication
//
// **Detection**: Company certificate contains LocationRoles extension
//
// **Authentication Logic**:
// - Full certificate chain validation supported
// - Intermediate certificates allowed between user and root CA
// - Enhanced LocationRoles extension (1.3.6.1.4.1.59193.2.1) for granular permissions
// - Backwards compatible with V1 user certificates in V2 chains
//
// **Certificate Chain**: User Cert → [Intermediate CAs] → Root CA (max 10 levels)
//
// **Extensions Used**:
// - LocationRoles extension for fine-grained location-based role mapping
// - Support for V1 extensions in mixed chains
//
// **Security Model**: Full PKI with delegation capabilities
//
// **Admin Delegation**: Admins can create intermediate CAs with restricted scopes

// ## Implementation Details
//
// **Version Detection**:
// ```go
// companyCert := GetCACertificate(ctx, user.CompanyId, gormDB, redisClient)
// if companyCert == nil {
//     // v0: Legacy mode
// } else {
//     _, err := certificate.GetLocationRolesFromCertificate(companyX509Cert)
//     if errors.Is(err, certificate.ERR_RoleLocation_Not_Found) {
//         // v1: Basic mode
//     } else {
//         // v2: Advanced mode
//     }
// }
// ```
//
// **Validation Functions**:
// - v0: Database-only validation via IsCompanyOwner()
// - v1: certificate.ValidateX509UserCertificate(userCert, companyCert, nil)
// - v2: ValidateUserCertificateChain(ctx, user, gormDB, redisClient)

// ## Security Considerations
//
// **Backwards Compatibility vs Security**:
// - v0 maintains access for legacy users but provides minimal security
// - v1 provides basic certificate security with simple validation
// - v2 enables full PKI security with granular permission control
//
// **Migration Safety**:
// - Users are never locked out during version transitions
// - Company owners retain access across all versions
// - Certificate validation failures gracefully fall back when appropriate
//
// **Attack Prevention**:
// - Mixed chain validation prevents downgrade attacks
// - Extension presence enforces appropriate validation level
// - Certificate chain depth limits prevent DoS via deep hierarchies

// ## Operational Impact
//
// **Performance**:
// - v0: Fastest (database lookup only)
// - v1: Medium (single certificate validation)
// - v2: Slower (full chain validation with multiple certificates)
//
// **Management Complexity**:
// - v0: Minimal (database-based permissions only)
// - v1: Moderate (direct certificate management)
// - v2: Complex (full PKI with intermediates and delegation)
//
// **Scalability**:
// - v0: Limited by database performance
// - v1: Scales well for simple hierarchies
// - v2: Scales to enterprise-level complex hierarchies with thousands of locations

// ## Version Upgrade Path
//
// **v0 → v1 Migration**:
// 1. Generate company CA certificate (without LocationRoles extension)
// 2. Issue user certificates with Role and Location extensions
// 3. Users automatically use v1 validation on next authentication
//
// **v1 → v2 Migration**:
// 1. Generate new company CA certificate with LocationRoles extension
// 2. Existing v1 user certificates continue working (backwards compatibility)
// 3. New user certificates use v2 LocationRoles extension
// 4. Gradual migration as certificates are renewed
//
// **Rollback Support**:
// - v2 → v1: Remove LocationRoles extension from company certificate
// - v1 → v0: Remove company certificate (not recommended for security)

