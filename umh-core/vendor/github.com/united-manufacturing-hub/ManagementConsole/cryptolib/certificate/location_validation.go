// Package certificate implements safe location string parsing and validation
// for the V2 certificate system to prevent injection attacks and enforce security constraints.
package certificate

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Location parsing security constraints defined in constants for centralized management
const (
	// MaxLocationDepthLimit defines the maximum number of location hierarchy levels.
	//
	// **Value**: 50 levels
	//
	// **Reasoning**: Prevents DoS attacks through excessively deep hierarchies while
	// allowing reasonable industrial organizational depth. Balances flexibility with
	// performance and security.
	//
	// **Attack Prevention**:
	// - DoS through deep recursion in permission resolution
	// - Memory exhaustion through very long location strings
	// - Performance degradation in hierarchical lookups
	MaxLocationDepthLimit = 50

	// MaxLocationSegmentLength defines the maximum length of a single location segment.
	//
	// **Value**: 100 characters
	//
	// **Reasoning**: Allows descriptive location names while preventing buffer overflow
	// attacks and maintaining reasonable certificate sizes.
	//
	// **Examples**: "production-line-A-welding-station-5" (36 chars) is reasonable
	MaxLocationSegmentLength = 100

	// MaxLocationTotalLength defines the maximum total length of a location string.
	//
	// **Value**: 2000 characters
	//
	// **Reasoning**: Prevents extremely long location strings that could:
	// - Cause certificate size issues
	// - Enable DoS attacks through memory exhaustion
	// - Impact parsing performance
	MaxLocationTotalLength = 2000

	// LocationSegmentMinLength defines the minimum length of a location segment.
	//
	// **Value**: 1 character
	//
	// **Reasoning**: Prevents empty segments which could be used in path traversal
	// attacks or cause parsing ambiguities.
	LocationSegmentMinLength = 1
)

// Regex patterns for location validation
var (
	// locationSegmentPattern matches valid characters in location segments.
	//
	// **Allowed characters**:
	// - Alphanumeric (a-z, A-Z, 0-9)
	// - Hyphens (-) for multi-word names
	// - Underscores (_) for programming-style names
	// - Unicode letters and numbers for international support
	//
	// **Explicitly prohibited**:
	// - Dots (.) - reserved as hierarchy separator
	// - Forward/back slashes (/, \) - prevent path traversal
	// - Null bytes (\x00) - prevent injection attacks
	// - Control characters - prevent terminal/protocol attacks
	// - Spaces - avoid parsing ambiguities (use - or _ instead)
	locationSegmentPattern = regexp.MustCompile(`^[\p{L}\p{N}_-]+$`)

	// pathTraversalPattern detects path traversal attack patterns
	pathTraversalPattern = regexp.MustCompile(`\.\.`)

	// absolutePathPattern detects absolute path attempts
	absolutePathPattern = regexp.MustCompile(`^[/\\]`)
)

// LocationValidationError represents errors from location string validation
type LocationValidationError struct {
	Location string
	Reason   string
	Position int // -1 if not applicable to a specific position
}

func (e LocationValidationError) Error() string {
	if e.Position >= 0 {
		return fmt.Sprintf("invalid location '%s': %s (at position %d)", e.Location, e.Reason, e.Position)
	}
	return fmt.Sprintf("invalid location '%s': %s", e.Location, e.Reason)
}

// SafeLocationSegments represents the validated segments of a location path
type SafeLocationSegments struct {
	Original string   // Original input for reference
	Segments []string // Validated segments
	Depth    int      // Number of hierarchy levels
}

// SafeParseLocationString securely parses and validates a location string.
// This function implements comprehensive security validation to prevent:
// - Path traversal attacks (../, ../../, etc.)
// - Injection attacks through special characters
// - DoS attacks through excessively deep or long hierarchies
// - Buffer overflow attempts through oversized segments
//
// **Validation Rules**:
// 1. Total length must not exceed MaxLocationTotalLength
// 2. Hierarchy depth must not exceed MaxLocationDepthLimit
// 3. Each segment must be 1-100 characters
// 4. Segments can only contain alphanumeric, hyphens, underscores, and Unicode letters/numbers
// 5. No path traversal patterns (..) allowed
// 6. No absolute path patterns (/, \) allowed
// 7. No empty segments (double dots like "a..b")
// 8. No control characters or null bytes
//
// **Usage**:
//
//	segments, err := SafeParseLocationString("umh.cologne.factory.line1")
//	if err != nil {
//	    return fmt.Errorf("invalid location: %w", err)
//	}
//	// Use segments.Segments for safe processing
//
// Parameters:
//   - location: The dot-separated location string to validate
//
// Returns:
//   - *SafeLocationSegments: Validated segments and metadata
//   - error: Detailed validation error if location is invalid
func SafeParseLocationString(location string) (*SafeLocationSegments, error) {
	// Handle wildcard case - special location that bypasses normal validation
	if location == LocationWildcard {
		return &SafeLocationSegments{
			Original: location,
			Segments: []string{LocationWildcard},
			Depth:    1,
		}, nil
	}

	// Validate input is not empty
	if location == "" {
		return nil, LocationValidationError{
			Location: location,
			Reason:   "location cannot be empty",
			Position: -1,
		}
	}

	// Validate total length
	if len(location) > MaxLocationTotalLength {
		return nil, LocationValidationError{
			Location: location,
			Reason:   fmt.Sprintf("location exceeds maximum length of %d characters", MaxLocationTotalLength),
			Position: -1,
		}
	}

	// Check for path traversal patterns
	if pathTraversalPattern.MatchString(location) {
		return nil, LocationValidationError{
			Location: location,
			Reason:   "path traversal patterns (..) are not allowed",
			Position: strings.Index(location, ".."),
		}
	}

	// Check for absolute path patterns
	if absolutePathPattern.MatchString(location) {
		return nil, LocationValidationError{
			Location: location,
			Reason:   "absolute paths (starting with / or \\) are not allowed",
			Position: 0,
		}
	}

	// Check for control characters and null bytes
	for i, r := range location {
		if unicode.IsControl(r) || r == 0 {
			return nil, LocationValidationError{
				Location: location,
				Reason:   fmt.Sprintf("control character or null byte not allowed (found U+%04X)", r),
				Position: i,
			}
		}
	}

	// Validate UTF-8 encoding
	if !utf8.ValidString(location) {
		return nil, LocationValidationError{
			Location: location,
			Reason:   "location must be valid UTF-8",
			Position: -1,
		}
	}

	// Split into segments and validate each one
	segments := strings.Split(location, LocationPathSeparator)

	// Validate hierarchy depth
	if len(segments) > MaxLocationDepthLimit {
		return nil, LocationValidationError{
			Location: location,
			Reason:   fmt.Sprintf("location depth exceeds maximum of %d levels", MaxLocationDepthLimit),
			Position: -1,
		}
	}

	// Validate each segment
	for i, segment := range segments {
		if err := validateLocationSegment(segment, i); err != nil {
			// Wrap the segment error with location context
			if segErr, ok := err.(LocationValidationError); ok {
				return nil, LocationValidationError{
					Location: location,
					Reason:   fmt.Sprintf("segment %d: %s", i+1, segErr.Reason),
					Position: segErr.Position,
				}
			}
			return nil, err
		}
	}

	return &SafeLocationSegments{
		Original: location,
		Segments: segments,
		Depth:    len(segments),
	}, nil
}

// validateLocationSegment validates a single location segment
func validateLocationSegment(segment string, segmentIndex int) error {
	// Check length constraints
	if len(segment) < LocationSegmentMinLength {
		return LocationValidationError{
			Location: segment,
			Reason:   "segment cannot be empty",
			Position: segmentIndex,
		}
	}

	if len(segment) > MaxLocationSegmentLength {
		return LocationValidationError{
			Location: segment,
			Reason:   fmt.Sprintf("segment exceeds maximum length of %d characters", MaxLocationSegmentLength),
			Position: segmentIndex,
		}
	}

	// Trim whitespace and verify no whitespace remains
	trimmed := strings.TrimSpace(segment)
	if trimmed != segment {
		return LocationValidationError{
			Location: segment,
			Reason:   "segment cannot start or end with whitespace",
			Position: segmentIndex,
		}
	}

	// Check for internal whitespace
	if strings.Contains(segment, " ") || strings.Contains(segment, "\t") {
		return LocationValidationError{
			Location: segment,
			Reason:   "segment cannot contain whitespace (use - or _ instead)",
			Position: strings.IndexAny(segment, " \t"),
		}
	}

	// Validate character pattern
	if !locationSegmentPattern.MatchString(segment) {
		return LocationValidationError{
			Location: segment,
			Reason:   "segment contains invalid characters (only alphanumeric, hyphens, underscores, and Unicode letters/numbers allowed)",
			Position: segmentIndex,
		}
	}

	return nil
}

// SafeParseLocationCandidates safely generates hierarchical location candidates
// for permission resolution. This function is used by GetRoleForLocation to
// build the list of locations to check from most specific to least specific.
//
// **Security Note**: This function only operates on already-validated location
// segments from SafeParseLocationString, ensuring no additional validation
// is required.
//
// Parameters:
//   - segments: Pre-validated location segments from SafeParseLocationString
//
// Returns:
//   - []string: List of location candidates from most to least specific, plus wildcard
func SafeParseLocationCandidates(segments *SafeLocationSegments) []string {
	if len(segments.Segments) == 0 {
		return []string{LocationWildcard}
	}

	// Handle wildcard case
	if len(segments.Segments) == 1 && segments.Segments[0] == LocationWildcard {
		return []string{LocationWildcard}
	}

	candidates := make([]string, 0, len(segments.Segments)+1)

	// Build candidates from most specific to least specific
	for i := len(segments.Segments); i > 0; i-- {
		candidate := strings.Join(segments.Segments[:i], LocationPathSeparator)
		candidates = append(candidates, candidate)
	}

	// Add wildcard as final fallback
	candidates = append(candidates, LocationWildcard)

	return candidates
}

// ValidateLocationRoles validates a LocationRoles map ensuring all location strings
// are safely parsed and all roles are valid. This function should be used before
// adding LocationRoles to certificates.
//
// Parameters:
//   - locationRoles: Map of location strings to roles to validate
//
// Returns:
//   - error: nil if all locations and roles are valid, descriptive error otherwise
func ValidateLocationRoles(locationRoles LocationRoles) error {
	if len(locationRoles) == 0 {
		return errors.New("locationRoles cannot be empty")
	}

	for location, role := range locationRoles {
		// Validate the role using centralized parseRole function
		_, err := parseRole(role)
		if err != nil {
			return fmt.Errorf("invalid role '%s' for location '%s': %w", role, location, err)
		}

		// Validate the location using safe parsing
		_, err = SafeParseLocationString(location)
		if err != nil {
			return fmt.Errorf("invalid location '%s': %w", location, err)
		}
	}

	return nil
}
