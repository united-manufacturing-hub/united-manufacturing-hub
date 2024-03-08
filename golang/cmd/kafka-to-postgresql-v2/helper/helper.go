package helper

import (
	"database/sql"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
)

func InitLogging() {
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION") //nolint:errcheck
	_ = logger.New(logLevel)
}

func InitTestLogging() {
	_ = logger.New("DEVELOPMENT")
}

// Uint64PtrToNullInt64
// Function to convert *uint64 to a value that can be inserted into the DB.
// It returns an sql.NullInt64, which is a struct that can hold an int64 value and a flag indicating if the value is valid.
func Uint64PtrToNullInt64(val *uint64) sql.NullInt64 {
	if val == nil {
		return sql.NullInt64{Valid: false}
	}
	// Note: This assumes the uint64 value can safely be converted to an int64.
	v := int64(*val)

	return sql.NullInt64{Valid: true, Int64: v}
}

func IntToUint64Ptr(val int) *uint64 {
	v := uint64(val)
	return &v
}

func IntToInt64Ptr(val int) *int64 {
	v := int64(val)
	return &v
}

// StringToPtr you probably don't need this outside of tests
// Checkout StringPtrToNullString for a more useful function
func StringToPtr(val string) *string {
	return &val
}

func StringPtrToNullString(val *string) sql.NullString {
	if val == nil {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{Valid: true, String: *val}
}

func StringToNullString(val string) sql.NullString {
	if val == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{Valid: true, String: val}
}
