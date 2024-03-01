package pgerror

import (
	"github.com/lib/pq"
)

// Class 00 - Successful Completion

// SuccessfulCompletion checks if the error is of code 00000
func SuccessfulCompletion(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "00000" {
		return pqerr
	}
	return nil
}

// Class 01 - Warning

// Warning checks if the error is of code 01000
func Warning(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "01000" {
		return pqerr
	}
	return nil
}

// DynamicResultSetsReturned checks if the error is of code 0100C
func DynamicResultSetsReturned(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "0100C" {
		return pqerr
	}
	return nil
}

// ImplicitZeroBitPadding checks if the error is of code 01008
func ImplicitZeroBitPadding(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "01008" {
		return pqerr
	}
	return nil
}

// NullValueEliminatedInSetFunction checks if the error is of code 01003
func NullValueEliminatedInSetFunction(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "01003" {
		return pqerr
	}
	return nil
}

// PrivilegeNotGranted checks if the error is of code 01007
func PrivilegeNotGranted(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "01007" {
		return pqerr
	}
	return nil
}

// PrivilegeNotRevoked checks if the error is of code 01006
func PrivilegeNotRevoked(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "01006" {
		return pqerr
	}
	return nil
}

// StringDataRightTruncation checks if the error is of code 01004
func StringDataRightTruncation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		(pqerr.Code == "01004" || pqerr.Code == "22001") {
		return pqerr
	}
	return nil
}

// DeprecatedFeature checks if the error is of code 01P01
func DeprecatedFeature(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "01P01" {
		return pqerr
	}
	return nil
}

// Class 02 - No Data (this is also a warning class per the SQL standard)

// NoData checks if the error is of code 02000
func NoData(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "02000" {
		return pqerr
	}
	return nil
}

// NoAdditionalDynamicResultSetsReturned checks if the error is of code 02001
func NoAdditionalDynamicResultSetsReturned(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "02001" {
		return pqerr
	}
	return nil
}

// Class 03 - SQL Statement Not Yet Complete

// SQLStatementNotYetComplete checks if the error is of code 03000
func SQLStatementNotYetComplete(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "03000" {
		return pqerr
	}
	return nil
}

// Class 08 - Connection Exception

// ConnectionException checks if the error is of code 08000
func ConnectionException(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "08000" {
		return pqerr
	}
	return nil
}

// ConnectionDoesNotExist checks if the error is of code 08003
func ConnectionDoesNotExist(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "08003" {
		return pqerr
	}
	return nil
}

// ConnectionFailure checks if the error is of code 08006
func ConnectionFailure(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "08006" {
		return pqerr
	}
	return nil
}

// SQLclientUnableToEstablishSQLconnection checks if the error is of code 08001
func SQLclientUnableToEstablishSQLconnection(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "08001" {
		return pqerr
	}
	return nil
}

// SQLserverRejectedEstablishmentOfSQLconnection checks if the error is of code 08004
func SQLserverRejectedEstablishmentOfSQLconnection(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "08004" {
		return pqerr
	}
	return nil
}

// TransactionResolutionUnknown checks if the error is of code 08007
func TransactionResolutionUnknown(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "08007" {
		return pqerr
	}
	return nil
}

// ProtocolViolation checks if the error is of code 08P01
func ProtocolViolation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "08P01" {
		return pqerr
	}
	return nil
}

// Class 09 - Triggered Action Exception

// TriggeredActionException checks if the error is of code 09000
func TriggeredActionException(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "09000" {
		return pqerr
	}
	return nil
}

// Class 0A - Feature Not Supported

// FeatureNotSupported checks if the error is of code 0A000
func FeatureNotSupported(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "0A000" {
		return pqerr
	}
	return nil
}

// Class 0B - Invalid Transaction Initiation

// InvalidTransactionInitiation checks if the error is of code 0B000
func InvalidTransactionInitiation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "0B000" {
		return pqerr
	}
	return nil
}

// Class 0F - Locator Exception

// LocatorException checks if the error is of code 0F000
func LocatorException(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "0F000" {
		return pqerr
	}
	return nil
}

// InvalidLocatorSpecification checks if the error is of code 0F001
func InvalidLocatorSpecification(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "0F001" {
		return pqerr
	}
	return nil
}

// Class 0L - Invalid Grantor

// InvalidGrantor checks if the error is of code 0L000
func InvalidGrantor(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "0L000" {
		return pqerr
	}
	return nil
}

// InvalidGrantOperation checks if the error is of code 0LP01
func InvalidGrantOperation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "0LP01" {
		return pqerr
	}
	return nil
}

// Class 0P - Invalid Role Specification

// InvalidRoleSpecification checks if the error is of code 0P000
func InvalidRoleSpecification(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "0P000" {
		return pqerr
	}
	return nil
}

// Class 0Z - Diagnostics Exception

// DiagnosticsException checks if the error is of code 0Z000
func DiagnosticsException(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "0Z000" {
		return pqerr
	}
	return nil
}

// StackedDiagnosticsAccessedWithoutActiveHandler checks if the error is of code 0Z002
func StackedDiagnosticsAccessedWithoutActiveHandler(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "0Z002" {
		return pqerr
	}
	return nil
}

// Class 20 - Case Not Found

// CaseNotFound checks if the error is of code 20000
func CaseNotFound(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "20000" {
		return pqerr
	}
	return nil
}

// Class 21 - Cardinality Violation

// CardinalityViolation checks if the error is of code 21000
func CardinalityViolation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "21000" {
		return pqerr
	}
	return nil
}

// Class 22 - Data Exception

// DataException checks if the error is of code 22000
func DataException(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22000" {
		return pqerr
	}
	return nil
}

// ArraySubscriptError checks if the error is of code 2202E
func ArraySubscriptError(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2202E" {
		return pqerr
	}
	return nil
}

// CharacterNotInRepertoire checks if the error is of code 22021
func CharacterNotInRepertoire(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22021" {
		return pqerr
	}
	return nil
}

// DatetimeFieldOverflow checks if the error is of code 22008
func DatetimeFieldOverflow(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22008" {
		return pqerr
	}
	return nil
}

// DivisionByZero checks if the error is of code 22012
func DivisionByZero(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22012" {
		return pqerr
	}
	return nil
}

// ErrorInAssignment checks if the error is of code 22005
func ErrorInAssignment(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22005" {
		return pqerr
	}
	return nil
}

// EscapeCharacterConflict checks if the error is of code 2200B
func EscapeCharacterConflict(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2200B" {
		return pqerr
	}
	return nil
}

// IndicatorOverflow checks if the error is of code 22022
func IndicatorOverflow(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22022" {
		return pqerr
	}
	return nil
}

// IntervalFieldOverflow checks if the error is of code 22015
func IntervalFieldOverflow(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22015" {
		return pqerr
	}
	return nil
}

// InvalidArgumentForLogarithm checks if the error is of code 2201E
func InvalidArgumentForLogarithm(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2201E" {
		return pqerr
	}
	return nil
}

// InvalidArgumentForNtileFunction checks if the error is of code 22014
func InvalidArgumentForNtileFunction(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22014" {
		return pqerr
	}
	return nil
}

// InvalidArgumentForNthValueFunction checks if the error is of code 22016
func InvalidArgumentForNthValueFunction(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22016" {
		return pqerr
	}
	return nil
}

// InvalidArgumentForPowerFunction checks if the error is of code 2201F
func InvalidArgumentForPowerFunction(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2201F" {
		return pqerr
	}
	return nil
}

// InvalidArgumentForWidthBucketFunction checks if the error is of code 2201G
func InvalidArgumentForWidthBucketFunction(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2201G" {
		return pqerr
	}
	return nil
}

// InvalidCharacterValueForCast checks if the error is of code 22018
func InvalidCharacterValueForCast(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22018" {
		return pqerr
	}
	return nil
}

// InvalidDatetimeFormat checks if the error is of code 22007
func InvalidDatetimeFormat(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22007" {
		return pqerr
	}
	return nil
}

// InvalidEscapeCharacter checks if the error is of code 22019
func InvalidEscapeCharacter(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22019" {
		return pqerr
	}
	return nil
}

// InvalidEscapeOctet checks if the error is of code 2200D
func InvalidEscapeOctet(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2200D" {
		return pqerr
	}
	return nil
}

// InvalidEscapeSequence checks if the error is of code 22025
func InvalidEscapeSequence(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22025" {
		return pqerr
	}
	return nil
}

// NonstandardUseOfEscapeCharacter checks if the error is of code 22P06
func NonstandardUseOfEscapeCharacter(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22P06" {
		return pqerr
	}
	return nil
}

// InvalidIndicatorParameterValue checks if the error is of code 22010
func InvalidIndicatorParameterValue(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22010" {
		return pqerr
	}
	return nil
}

// InvalidParameterValue checks if the error is of code 22023
func InvalidParameterValue(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22023" {
		return pqerr
	}
	return nil
}

// InvalidRegularExpression checks if the error is of code 2201B
func InvalidRegularExpression(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2201B" {
		return pqerr
	}
	return nil
}

// InvalidRowCountInLimitClause checks if the error is of code 2201W
func InvalidRowCountInLimitClause(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2201W" {
		return pqerr
	}
	return nil
}

// InvalidRowCountInResultOffsetClause checks if the error is of code 2201X
func InvalidRowCountInResultOffsetClause(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2201X" {
		return pqerr
	}
	return nil
}

// InvalidTimeZoneDisplacementValue checks if the error is of code 22009
func InvalidTimeZoneDisplacementValue(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22009" {
		return pqerr
	}
	return nil
}

// InvalidUseOfEscapeCharacter checks if the error is of code 2200C
func InvalidUseOfEscapeCharacter(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2200C" {
		return pqerr
	}
	return nil
}

// MostSpecificTypeMismatch checks if the error is of code 2200G
func MostSpecificTypeMismatch(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2200G" {
		return pqerr
	}
	return nil
}

// NullValueNotAllowed checks if the error is of code 22004
func NullValueNotAllowed(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		(pqerr.Code == "22004" || pqerr.Code == "39004") {
		return pqerr
	}
	return nil
}

// NullValueNoIndicatorParameter checks if the error is of code 22002
func NullValueNoIndicatorParameter(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22002" {
		return pqerr
	}
	return nil
}

// NumericValueOutOfRange checks if the error is of code 22003
func NumericValueOutOfRange(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22003" {
		return pqerr
	}
	return nil
}

// StringDataLengthMismatch checks if the error is of code 22026
func StringDataLengthMismatch(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22026" {
		return pqerr
	}
	return nil
}

// SubstringError checks if the error is of code 22011
func SubstringError(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22011" {
		return pqerr
	}
	return nil
}

// TrimError checks if the error is of code 22027
func TrimError(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22027" {
		return pqerr
	}
	return nil
}

// UnterminatedCString checks if the error is of code 22024
func UnterminatedCString(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22024" {
		return pqerr
	}
	return nil
}

// ZeroLengthCharacterString checks if the error is of code 2200F
func ZeroLengthCharacterString(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2200F" {
		return pqerr
	}
	return nil
}

// FloatingPointException checks if the error is of code 22P01
func FloatingPointException(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22P01" {
		return pqerr
	}
	return nil
}

// InvalidTextRepresentation checks if the error is of code 22P02
func InvalidTextRepresentation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22P02" {
		return pqerr
	}
	return nil
}

// InvalidBinaryRepresentation checks if the error is of code 22P03
func InvalidBinaryRepresentation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22P03" {
		return pqerr
	}
	return nil
}

// BadCopyFileFormat checks if the error is of code 22P04
func BadCopyFileFormat(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22P04" {
		return pqerr
	}
	return nil
}

// UntranslatableCharacter checks if the error is of code 22P05
func UntranslatableCharacter(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "22P05" {
		return pqerr
	}
	return nil
}

// NotAnXMLDocument checks if the error is of code 2200L
func NotAnXMLDocument(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2200L" {
		return pqerr
	}
	return nil
}

// InvalidXMLDocument checks if the error is of code 2200M
func InvalidXMLDocument(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2200M" {
		return pqerr
	}
	return nil
}

// InvalidXMLContent checks if the error is of code 2200N
func InvalidXMLContent(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2200N" {
		return pqerr
	}
	return nil
}

// InvalidXMLComment checks if the error is of code 2200S
func InvalidXMLComment(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2200S" {
		return pqerr
	}
	return nil
}

// InvalidXMLProcessingInstruction checks if the error is of code 2200T
func InvalidXMLProcessingInstruction(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2200T" {
		return pqerr
	}
	return nil
}

// Class 23 - Integrity Constraint Violation

// IntegrityConstraintViolation checks if the error is of code 23000
func IntegrityConstraintViolation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "23000" {
		return pqerr
	}
	return nil
}

// RestrictViolation checks if the error is of code 23001
func RestrictViolation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "23001" {
		return pqerr
	}
	return nil
}

// NotNullViolation checks if the error is of code 23502
func NotNullViolation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "23502" {
		return pqerr
	}
	return nil
}

// ForeignKeyViolation checks if the error is of code 23503
func ForeignKeyViolation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "23503" {
		return pqerr
	}
	return nil
}

// UniqueViolation checks if the error is of code 23505
func UniqueViolation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "23505" {
		return pqerr
	}
	return nil
}

// CheckViolation checks if the error is of code 23514
func CheckViolation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "23514" {
		return pqerr
	}
	return nil
}

// ExclusionViolation checks if the error is of code 23P01
func ExclusionViolation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "23P01" {
		return pqerr
	}
	return nil
}

// Class 24 - Invalid Cursor State

// InvalidCursorState checks if the error is of code 24000
func InvalidCursorState(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "24000" {
		return pqerr
	}
	return nil
}

// Class 25 - Invalid Transaction State

// InvalidTransactionState checks if the error is of code 25000
func InvalidTransactionState(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "25000" {
		return pqerr
	}
	return nil
}

// ActiveSQLTransaction checks if the error is of code 25001
func ActiveSQLTransaction(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "25001" {
		return pqerr
	}
	return nil
}

// BranchTransactionAlreadyActive checks if the error is of code 25002
func BranchTransactionAlreadyActive(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "25002" {
		return pqerr
	}
	return nil
}

// HeldCursorRequiresSameIsolationLevel checks if the error is of code 25008
func HeldCursorRequiresSameIsolationLevel(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "25008" {
		return pqerr
	}
	return nil
}

// InappropriateAccessModeForBranchTransaction checks if the error is of code 25003
func InappropriateAccessModeForBranchTransaction(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "25003" {
		return pqerr
	}
	return nil
}

// InappropriateIsolationLevelForBranchTransaction checks if the error is of code 25004
func InappropriateIsolationLevelForBranchTransaction(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "25004" {
		return pqerr
	}
	return nil
}

// NoActiveSQLTransactionForBranchTransaction checks if the error is of code 25005
func NoActiveSQLTransactionForBranchTransaction(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "25005" {
		return pqerr
	}
	return nil
}

// ReadOnlySQLTransaction checks if the error is of code 25006
func ReadOnlySQLTransaction(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "25006" {
		return pqerr
	}
	return nil
}

// SchemaAndDataStatementMixingNotSupported checks if the error is of code 25007
func SchemaAndDataStatementMixingNotSupported(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "25007" {
		return pqerr
	}
	return nil
}

// NoActiveSQLTransaction checks if the error is of code 25P01
func NoActiveSQLTransaction(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "25P01" {
		return pqerr
	}
	return nil
}

// InFailedSQLTransaction checks if the error is of code 25P02
func InFailedSQLTransaction(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "25P02" {
		return pqerr
	}
	return nil
}

// Class 26 - Invalid SQL Statement Name

// InvalidSQLStatementName checks if the error is of code 26000
func InvalidSQLStatementName(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "26000" {
		return pqerr
	}
	return nil
}

// Class 27 - Triggered Data Change Violation

// TriggeredDataChangeViolation checks if the error is of code 27000
func TriggeredDataChangeViolation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "27000" {
		return pqerr
	}
	return nil
}

// Class 28 - Invalid Authorization Specification

// InvalidAuthorizationSpecification checks if the error is of code 28000
func InvalidAuthorizationSpecification(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "28000" {
		return pqerr
	}
	return nil
}

// InvalidPassword checks if the error is of code 28P01
func InvalidPassword(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "28P01" {
		return pqerr
	}
	return nil
}

// Class 2B - Dependent Privilege Descriptors Still Exist

// DependentPrivilegeDescriptorsStillExist checks if the error is of code 2B000
func DependentPrivilegeDescriptorsStillExist(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2B000" {
		return pqerr
	}
	return nil
}

// DependentObjectsStillExist checks if the error is of code 2BP01
func DependentObjectsStillExist(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2BP01" {
		return pqerr
	}
	return nil
}

// Class 2D - Invalid Transaction Termination

// InvalidTransactionTermination checks if the error is of code 2D000
func InvalidTransactionTermination(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2D000" {
		return pqerr
	}
	return nil
}

// Class 2F - SQL Routine Exception

// SQLRoutineException checks if the error is of code 2F000
func SQLRoutineException(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2F000" {
		return pqerr
	}
	return nil
}

// FunctionExecutedNoReturnStatement checks if the error is of code 2F005
func FunctionExecutedNoReturnStatement(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "2F005" {
		return pqerr
	}
	return nil
}

// ModifyingSQLDataNotPermitted checks if the error is of code 2F002
func ModifyingSQLDataNotPermitted(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		(pqerr.Code == "2F002" || pqerr.Code == "38002") {
		return pqerr
	}
	return nil
}

// ProhibitedSQLStatementAttempted checks if the error is of code 2F003
func ProhibitedSQLStatementAttempted(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		(pqerr.Code == "2F003" || pqerr.Code == "38003") {
		return pqerr
	}
	return nil
}

// ReadingSQLDataNotPermitted checks if the error is of code 2F004
func ReadingSQLDataNotPermitted(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		(pqerr.Code == "2F004" || pqerr.Code == "38004") {
		return pqerr
	}
	return nil
}

// Class 34 - Invalid Cursor Name

// InvalidCursorName checks if the error is of code 34000
func InvalidCursorName(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "34000" {
		return pqerr
	}
	return nil
}

// Class 38 - External Routine Exception

// ExternalRoutineException checks if the error is of code 38000
func ExternalRoutineException(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "38000" {
		return pqerr
	}
	return nil
}

// ContainingSQLNotPermitted checks if the error is of code 38001
func ContainingSQLNotPermitted(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "38001" {
		return pqerr
	}
	return nil
}

// Class 39 - External Routine Invocation Exception

// ExternalRoutineInvocationException checks if the error is of code 39000
func ExternalRoutineInvocationException(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "39000" {
		return pqerr
	}
	return nil
}

// InvalidSQLstateReturned checks if the error is of code 39001
func InvalidSQLstateReturned(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "39001" {
		return pqerr
	}
	return nil
}

// TriggerProtocolViolated checks if the error is of code 39P01
func TriggerProtocolViolated(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "39P01" {
		return pqerr
	}
	return nil
}

// SrfProtocolViolated checks if the error is of code 39P02
func SrfProtocolViolated(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "39P02" {
		return pqerr
	}
	return nil
}

// Class 3B - Savepoint Exception

// SavepointException checks if the error is of code 3B000
func SavepointException(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "3B000" {
		return pqerr
	}
	return nil
}

// InvalidSavepointSpecification checks if the error is of code 3B001
func InvalidSavepointSpecification(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "3B001" {
		return pqerr
	}
	return nil
}

// Class 3D - Invalid Catalog Name

// InvalidCatalogName checks if the error is of code 3D000
func InvalidCatalogName(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "3D000" {
		return pqerr
	}
	return nil
}

// Class 3F - Invalid Schema Name

// InvalidSchemaName checks if the error is of code 3F000
func InvalidSchemaName(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "3F000" {
		return pqerr
	}
	return nil
}

// Class 40 - Transaction Rollback

// TransactionRollback checks if the error is of code 40000
func TransactionRollback(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "40000" {
		return pqerr
	}
	return nil
}

// TransactionIntegrityConstraintViolation checks if the error is of code 40002
func TransactionIntegrityConstraintViolation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "40002" {
		return pqerr
	}
	return nil
}

// SerializationFailure checks if the error is of code 40001
func SerializationFailure(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "40001" {
		return pqerr
	}
	return nil
}

// StatementCompletionUnknown checks if the error is of code 40003
func StatementCompletionUnknown(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "40003" {
		return pqerr
	}
	return nil
}

// DeadlockDetected checks if the error is of code 40P01
func DeadlockDetected(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "40P01" {
		return pqerr
	}
	return nil
}

// Class 42 - Syntax Error or Access Rule Violation

// SyntaxErrorOrAccessRuleViolation checks if the error is of code 42000
func SyntaxErrorOrAccessRuleViolation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42000" {
		return pqerr
	}
	return nil
}

// SyntaxError checks if the error is of code 42601
func SyntaxError(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42601" {
		return pqerr
	}
	return nil
}

// InsufficientPrivilege checks if the error is of code 42501
func InsufficientPrivilege(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42501" {
		return pqerr
	}
	return nil
}

// CannotCoerce checks if the error is of code 42846
func CannotCoerce(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42846" {
		return pqerr
	}
	return nil
}

// GroupingError checks if the error is of code 42803
func GroupingError(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42803" {
		return pqerr
	}
	return nil
}

// WindowingError checks if the error is of code 42P20
func WindowingError(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P20" {
		return pqerr
	}
	return nil
}

// InvalidRecursion checks if the error is of code 42P19
func InvalidRecursion(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P19" {
		return pqerr
	}
	return nil
}

// InvalidForeignKey checks if the error is of code 42830
func InvalidForeignKey(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42830" {
		return pqerr
	}
	return nil
}

// InvalidName checks if the error is of code 42602
func InvalidName(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42602" {
		return pqerr
	}
	return nil
}

// NameTooLong checks if the error is of code 42622
func NameTooLong(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42622" {
		return pqerr
	}
	return nil
}

// ReservedName checks if the error is of code 42939
func ReservedName(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42939" {
		return pqerr
	}
	return nil
}

// DatatypeMismatch checks if the error is of code 42804
func DatatypeMismatch(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42804" {
		return pqerr
	}
	return nil
}

// IndeterminateDatatype checks if the error is of code 42P18
func IndeterminateDatatype(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P18" {
		return pqerr
	}
	return nil
}

// CollationMismatch checks if the error is of code 42P21
func CollationMismatch(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P21" {
		return pqerr
	}
	return nil
}

// IndeterminateCollation checks if the error is of code 42P22
func IndeterminateCollation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P22" {
		return pqerr
	}
	return nil
}

// WrongObjectType checks if the error is of code 42809
func WrongObjectType(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42809" {
		return pqerr
	}
	return nil
}

// UndefinedColumn checks if the error is of code 42703
func UndefinedColumn(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42703" {
		return pqerr
	}
	return nil
}

// UndefinedFunction checks if the error is of code 42883
func UndefinedFunction(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42883" {
		return pqerr
	}
	return nil
}

// UndefinedTable checks if the error is of code 42P01
func UndefinedTable(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P01" {
		return pqerr
	}
	return nil
}

// UndefinedParameter checks if the error is of code 42P02
func UndefinedParameter(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P02" {
		return pqerr
	}
	return nil
}

// UndefinedObject checks if the error is of code 42704
func UndefinedObject(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42704" {
		return pqerr
	}
	return nil
}

// DuplicateColumn checks if the error is of code 42701
func DuplicateColumn(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42701" {
		return pqerr
	}
	return nil
}

// DuplicateCursor checks if the error is of code 42P03
func DuplicateCursor(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P03" {
		return pqerr
	}
	return nil
}

// DuplicateDatabase checks if the error is of code 42P04
func DuplicateDatabase(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P04" {
		return pqerr
	}
	return nil
}

// DuplicateFunction checks if the error is of code 42723
func DuplicateFunction(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42723" {
		return pqerr
	}
	return nil
}

// DuplicatePreparedStatement checks if the error is of code 42P05
func DuplicatePreparedStatement(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P05" {
		return pqerr
	}
	return nil
}

// DuplicateSchema checks if the error is of code 42P06
func DuplicateSchema(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P06" {
		return pqerr
	}
	return nil
}

// DuplicateTable checks if the error is of code 42P07
func DuplicateTable(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P07" {
		return pqerr
	}
	return nil
}

// DuplicateAlias checks if the error is of code 42712
func DuplicateAlias(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42712" {
		return pqerr
	}
	return nil
}

// DuplicateObject checks if the error is of code 42710
func DuplicateObject(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42710" {
		return pqerr
	}
	return nil
}

// AmbiguousColumn checks if the error is of code 42702
func AmbiguousColumn(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42702" {
		return pqerr
	}
	return nil
}

// AmbiguousFunction checks if the error is of code 42725
func AmbiguousFunction(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42725" {
		return pqerr
	}
	return nil
}

// AmbiguousParameter checks if the error is of code 42P08
func AmbiguousParameter(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P08" {
		return pqerr
	}
	return nil
}

// AmbiguousAlias checks if the error is of code 42P09
func AmbiguousAlias(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P09" {
		return pqerr
	}
	return nil
}

// InvalidColumnReference checks if the error is of code 42P10
func InvalidColumnReference(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P10" {
		return pqerr
	}
	return nil
}

// InvalidColumnDefinition checks if the error is of code 42611
func InvalidColumnDefinition(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42611" {
		return pqerr
	}
	return nil
}

// InvalidCursorDefinition checks if the error is of code 42P11
func InvalidCursorDefinition(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P11" {
		return pqerr
	}
	return nil
}

// InvalidDatabaseDefinition checks if the error is of code 42P12
func InvalidDatabaseDefinition(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P12" {
		return pqerr
	}
	return nil
}

// InvalidFunctionDefinition checks if the error is of code 42P13
func InvalidFunctionDefinition(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P13" {
		return pqerr
	}
	return nil
}

// InvalidPreparedStatementDefinition checks if the error is of code 42P14
func InvalidPreparedStatementDefinition(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P14" {
		return pqerr
	}
	return nil
}

// InvalidSchemaDefinition checks if the error is of code 42P15
func InvalidSchemaDefinition(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P15" {
		return pqerr
	}
	return nil
}

// InvalidTableDefinition checks if the error is of code 42P16
func InvalidTableDefinition(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P16" {
		return pqerr
	}
	return nil
}

// InvalidObjectDefinition checks if the error is of code 42P17
func InvalidObjectDefinition(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "42P17" {
		return pqerr
	}
	return nil
}

// Class 44 - WITH CHECK OPTION Violation

// WithCheckOptionViolation checks if the error is of code 44000
func WithCheckOptionViolation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "44000" {
		return pqerr
	}
	return nil
}

// Class 53 - Insufficient Resources

// InsufficientResources checks if the error is of code 53000
func InsufficientResources(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "53000" {
		return pqerr
	}
	return nil
}

// DiskFull checks if the error is of code 53100
func DiskFull(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "53100" {
		return pqerr
	}
	return nil
}

// OutOfMemory checks if the error is of code 53200
func OutOfMemory(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "53200" {
		return pqerr
	}
	return nil
}

// TooManyConnections checks if the error is of code 53300
func TooManyConnections(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "53300" {
		return pqerr
	}
	return nil
}

// ConfigurationLimitExceeded checks if the error is of code 53400
func ConfigurationLimitExceeded(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "53400" {
		return pqerr
	}
	return nil
}

// Class 54 - Program Limit Exceeded

// ProgramLimitExceeded checks if the error is of code 54000
func ProgramLimitExceeded(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "54000" {
		return pqerr
	}
	return nil
}

// StatementTooComplex checks if the error is of code 54001
func StatementTooComplex(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "54001" {
		return pqerr
	}
	return nil
}

// TooManyColumns checks if the error is of code 54011
func TooManyColumns(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "54011" {
		return pqerr
	}
	return nil
}

// TooManyArguments checks if the error is of code 54023
func TooManyArguments(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "54023" {
		return pqerr
	}
	return nil
}

// Class 55 - Object Not In Prerequisite State

// ObjectNotInPrerequisiteState checks if the error is of code 55000
func ObjectNotInPrerequisiteState(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "55000" {
		return pqerr
	}
	return nil
}

// ObjectInUse checks if the error is of code 55006
func ObjectInUse(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "55006" {
		return pqerr
	}
	return nil
}

// CantChangeRuntimeParam checks if the error is of code 55P02
func CantChangeRuntimeParam(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "55P02" {
		return pqerr
	}
	return nil
}

// LockNotAvailable checks if the error is of code 55P03
func LockNotAvailable(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "55P03" {
		return pqerr
	}
	return nil
}

// Class 57 - Operator Intervention

// OperatorIntervention checks if the error is of code 57000
func OperatorIntervention(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "57000" {
		return pqerr
	}
	return nil
}

// QueryCanceled checks if the error is of code 57014
func QueryCanceled(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "57014" {
		return pqerr
	}
	return nil
}

// AdminShutdown checks if the error is of code 57P01
func AdminShutdown(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "57P01" {
		return pqerr
	}
	return nil
}

// CrashShutdown checks if the error is of code 57P02
func CrashShutdown(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "57P02" {
		return pqerr
	}
	return nil
}

// CannotConnectNow checks if the error is of code 57P03
func CannotConnectNow(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "57P03" {
		return pqerr
	}
	return nil
}

// DatabaseDropped checks if the error is of code 57P04
func DatabaseDropped(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "57P04" {
		return pqerr
	}
	return nil
}

// Class 58 - System Error (errors external to PostgreSQL itself)

// SystemError checks if the error is of code 58000
func SystemError(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "58000" {
		return pqerr
	}
	return nil
}

// IoError checks if the error is of code 58030
func IoError(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "58030" {
		return pqerr
	}
	return nil
}

// UndefinedFile checks if the error is of code 58P01
func UndefinedFile(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "58P01" {
		return pqerr
	}
	return nil
}

// DuplicateFile checks if the error is of code 58P02
func DuplicateFile(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "58P02" {
		return pqerr
	}
	return nil
}

// Class F0 - Configuration File Error

// ConfigFileError checks if the error is of code F0000
func ConfigFileError(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "F0000" {
		return pqerr
	}
	return nil
}

// LockFileExists checks if the error is of code F0001
func LockFileExists(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "F0001" {
		return pqerr
	}
	return nil
}

// Class HV - Foreign Data Wrapper Error (SQL/MED)

// FdwError checks if the error is of code HV000
func FdwError(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV000" {
		return pqerr
	}
	return nil
}

// FdwColumnNameNotFound checks if the error is of code HV005
func FdwColumnNameNotFound(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV005" {
		return pqerr
	}
	return nil
}

// FdwDynamicParameterValueNeeded checks if the error is of code HV002
func FdwDynamicParameterValueNeeded(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV002" {
		return pqerr
	}
	return nil
}

// FdwFunctionSequenceError checks if the error is of code HV010
func FdwFunctionSequenceError(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV010" {
		return pqerr
	}
	return nil
}

// FdwInconsistentDescriptorInformation checks if the error is of code HV021
func FdwInconsistentDescriptorInformation(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV021" {
		return pqerr
	}
	return nil
}

// FdwInvalidAttributeValue checks if the error is of code HV024
func FdwInvalidAttributeValue(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV024" {
		return pqerr
	}
	return nil
}

// FdwInvalidColumnName checks if the error is of code HV007
func FdwInvalidColumnName(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV007" {
		return pqerr
	}
	return nil
}

// FdwInvalidColumnNumber checks if the error is of code HV008
func FdwInvalidColumnNumber(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV008" {
		return pqerr
	}
	return nil
}

// FdwInvalidDataType checks if the error is of code HV004
func FdwInvalidDataType(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV004" {
		return pqerr
	}
	return nil
}

// FdwInvalidDataTypeDescriptors checks if the error is of code HV006
func FdwInvalidDataTypeDescriptors(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV006" {
		return pqerr
	}
	return nil
}

// FdwInvalidDescriptorFieldIdentifier checks if the error is of code HV091
func FdwInvalidDescriptorFieldIdentifier(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV091" {
		return pqerr
	}
	return nil
}

// FdwInvalidHandle checks if the error is of code HV00B
func FdwInvalidHandle(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV00B" {
		return pqerr
	}
	return nil
}

// FdwInvalidOptionIndex checks if the error is of code HV00C
func FdwInvalidOptionIndex(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV00C" {
		return pqerr
	}
	return nil
}

// FdwInvalidOptionName checks if the error is of code HV00D
func FdwInvalidOptionName(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV00D" {
		return pqerr
	}
	return nil
}

// FdwInvalidStringLengthOrBufferLength checks if the error is of code HV090
func FdwInvalidStringLengthOrBufferLength(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV090" {
		return pqerr
	}
	return nil
}

// FdwInvalidStringFormat checks if the error is of code HV00A
func FdwInvalidStringFormat(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV00A" {
		return pqerr
	}
	return nil
}

// FdwInvalidUseOfNullPointer checks if the error is of code HV009
func FdwInvalidUseOfNullPointer(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV009" {
		return pqerr
	}
	return nil
}

// FdwTooManyHandles checks if the error is of code HV014
func FdwTooManyHandles(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV014" {
		return pqerr
	}
	return nil
}

// FdwOutOfMemory checks if the error is of code HV001
func FdwOutOfMemory(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV001" {
		return pqerr
	}
	return nil
}

// FdwNoSchemas checks if the error is of code HV00P
func FdwNoSchemas(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV00P" {
		return pqerr
	}
	return nil
}

// FdwOptionNameNotFound checks if the error is of code HV00J
func FdwOptionNameNotFound(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV00J" {
		return pqerr
	}
	return nil
}

// FdwReplyHandle checks if the error is of code HV00K
func FdwReplyHandle(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV00K" {
		return pqerr
	}
	return nil
}

// FdwSchemaNotFound checks if the error is of code HV00Q
func FdwSchemaNotFound(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV00Q" {
		return pqerr
	}
	return nil
}

// FdwTableNotFound checks if the error is of code HV00R
func FdwTableNotFound(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV00R" {
		return pqerr
	}
	return nil
}

// FdwUnableToCreateExecution checks if the error is of code HV00L
func FdwUnableToCreateExecution(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV00L" {
		return pqerr
	}
	return nil
}

// FdwUnableToCreateReply checks if the error is of code HV00M
func FdwUnableToCreateReply(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV00M" {
		return pqerr
	}
	return nil
}

// FdwUnableToEstablishConnection checks if the error is of code HV00N
func FdwUnableToEstablishConnection(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "HV00N" {
		return pqerr
	}
	return nil
}

// Class P0 - PL/pgSQL Error

// PlpgsqlError checks if the error is of code P0000
func PlpgsqlError(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "P0000" {
		return pqerr
	}
	return nil
}

// RaiseException checks if the error is of code P0001
func RaiseException(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "P0001" {
		return pqerr
	}
	return nil
}

// NoDataFound checks if the error is of code P0002
func NoDataFound(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "P0002" {
		return pqerr
	}
	return nil
}

// TooManyRows checks if the error is of code P0003
func TooManyRows(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "P0003" {
		return pqerr
	}
	return nil
}

// Class XX - Internal Error

// InternalError checks if the error is of code XX000
func InternalError(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "XX000" {
		return pqerr
	}
	return nil
}

// DataCorrupted checks if the error is of code XX001
func DataCorrupted(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "XX001" {
		return pqerr
	}
	return nil
}

// IndexCorrupted checks if the error is of code XX002
func IndexCorrupted(err error) *pq.Error {
	if pqerr, ok := err.(*pq.Error); ok &&
		pqerr.Code == "XX002" {
		return pqerr
	}
	return nil
}
