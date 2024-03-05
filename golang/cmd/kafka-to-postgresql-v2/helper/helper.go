package helper

import (
	"github.com/pashagolub/pgxmock/v3"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"go.uber.org/zap"
)

func InitLogging() {
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION") //nolint:errcheck
	_ = logger.New(logLevel)
}

func InitTestLogging() {
	_ = logger.New("DEVELOPMENT")
}

// Uint64PtrToInt64Ptr
// Function to convert *uint64 to a value that can be inserted into the DB.
// It returns an interface{} which can either be a nil (representing SQL NULL)
// or an int64 value (if conversion is within range and not nil).
func Uint64PtrToInt64Ptr(val *uint64) *int64 {
	if val == nil {
		return nil
	}
	// Note: This assumes the uint64 value can safely be converted to an int64.
	v := int64(*val)
	return &v
}

func IntToUint64Ptr(val int) *uint64 {
	v := uint64(val)
	return &v
}

func IntToInt64Ptr(val int) *int64 {
	v := int64(val)
	return &v
}

func StringToPtr(val string) *string {
	return &val
}

type uint64PtrMatcher struct {
	expected *uint64
}

func (m uint64PtrMatcher) Match(v interface{}) bool {
	zap.S().Debugf("Comparing %v to %v", m.expected, v)
	// Type assert the value to a pointer to uint64.
	ptr, ok := v.(*uint64)
	if !ok {
		zap.S().Debugf("Expected *uint64, got %T", v)
		return false
	}
	// If both are nil, consider it a match.
	if m.expected == nil && ptr == nil {
		zap.S().Debugf("Matched nils")
		return true
	}
	// If one is nil but not the other, it's not a match.
	if m.expected == nil || ptr == nil {
		zap.S().Debugf("One is nil, the other is not")
		return false
	}
	// Compare the values pointed to.
	zap.S().Debugf("%d == %d", *m.expected, *ptr)
	return *m.expected == *ptr
}

// Helper function to create a new uint64PtrMatcher.
func MatchUint64Ptr(expected *uint64) pgxmock.Argument {
	return uint64PtrMatcher{expected}
}

type int64PtrMatcher struct {
	expected *int64
}

func (m int64PtrMatcher) Match(v interface{}) bool {
	zap.S().Debugf("Comparing %v to %v", m.expected, v)
	// Type assert the value to a pointer to uint64.
	ptr, ok := v.(*int64)
	if !ok {
		zap.S().Debugf("Expected *uint64, got %T", v)
		return false
	}
	// If both are nil, consider it a match.
	if m.expected == nil && ptr == nil {
		zap.S().Debugf("Matched nils")
		return true
	}
	// If one is nil but not the other, it's not a match.
	if m.expected == nil || ptr == nil {
		zap.S().Debugf("One is nil, the other is not")
		return false
	}
	// Compare the values pointed to.
	zap.S().Debugf("%d == %d", *m.expected, *ptr)
	return *m.expected == *ptr
}

// Helper function to create a new int64PtrMatcher.
func MatchInt64Ptr(expected *int64) pgxmock.Argument {
	return int64PtrMatcher{expected}
}

type stringPtrMatcher struct {
	expected *string
}

func (m stringPtrMatcher) Match(v interface{}) bool {
	zap.S().Debugf("Comparing %v to %v", m.expected, v)
	// Type assert the value to a pointer to ustring.
	ptr, ok := v.(*string)
	if !ok {
		zap.S().Debugf("Expected *ustring, got %T", v)
		return false
	}
	// If both are nil, consider it a match.
	if m.expected == nil && ptr == nil {
		zap.S().Debugf("Matched nils")
		return true
	}
	// If one is nil but not the other, it's not a match.
	if m.expected == nil || ptr == nil {
		zap.S().Debugf("One is nil, the other is not")
		return false
	}
	// Compare the values pointed to.
	zap.S().Debugf("%d == %d", *m.expected, *ptr)
	return *m.expected == *ptr
}

// Helper function to create a new stringPtrMatcher.
func MatchstringPtr(expected *string) pgxmock.Argument {
	return stringPtrMatcher{expected}
}
