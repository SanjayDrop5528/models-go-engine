// Package domain provides data transfer objects, domain models, and core types
// for Dataset Studio query definitions, custom columns, joins, aggregations,
// filter parameters, and execution modes.
//
// File: errors.go
// Usage:
//   This file defines the strongly-typed error taxonomy, error codes, and structured error
//   container for dataset operations across planning, validation, compiling, and execution.
//   It ensures that callers receive standardized machine-readable codes and context details
//   regardless of whether an error originated from AST validation or database execution.
package domain

import (
	"fmt"
)

// DataSetErrorCode represents a strongly-typed domain error code.
type DataSetErrorCode string

const (
	ErrDataSetNotFound           DataSetErrorCode = "DATASET_NOT_FOUND"
	ErrModelNotFound             DataSetErrorCode = "MODEL_NOT_FOUND"
	ErrFieldNotFound             DataSetErrorCode = "FIELD_NOT_FOUND"
	ErrFunctionNotFound          DataSetErrorCode = "FUNCTION_NOT_FOUND"
	ErrFunctionInactive          DataSetErrorCode = "FUNCTION_INACTIVE"
	ErrInvalidDataType           DataSetErrorCode = "INVALID_DATATYPE"
	ErrInvalidOperandCount       DataSetErrorCode = "INVALID_OPERAND_COUNT"
	ErrInvalidJoin               DataSetErrorCode = "INVALID_JOIN"
	ErrInvalidGroupBy            DataSetErrorCode = "INVALID_GROUP_BY"
	ErrInvalidFilter             DataSetErrorCode = "INVALID_FILTER"
	ErrInvalidFilterParameter    DataSetErrorCode = "INVALID_FILTER_PARAMETER"
	ErrMissingRequiredParameter  DataSetErrorCode = "MISSING_REQUIRED_PARAMETER"
	ErrInvalidParameterType      DataSetErrorCode = "INVALID_PARAMETER_TYPE"
	ErrUnsupportedDriver         DataSetErrorCode = "UNSUPPORTED_DRIVER"
	ErrUnsupportedFunction       DataSetErrorCode = "UNSUPPORTED_FUNCTION"
	ErrUnsupportedSaveMode       DataSetErrorCode = "UNSUPPORTED_SAVE_MODE"
	ErrPipelineCompilationFailed DataSetErrorCode = "PIPELINE_COMPILATION_FAILED"
	ErrPipelineExecutionFailed   DataSetErrorCode = "PIPELINE_EXECUTION_FAILED"
	ErrProcedureCreationFailed   DataSetErrorCode = "PROCEDURE_CREATION_FAILED"
	ErrFunctionCreationFailed    DataSetErrorCode = "FUNCTION_CREATION_FAILED"
)

// DataSetError wraps a typed error code and detailed message with metadata.
type DataSetError struct {
	Code     DataSetErrorCode `json:"code"`
	Message  string           `json:"message"`
	Details  map[string]any   `json:"details,omitempty"`
	CauseErr error            `json:"-"`
}

// Error formats the DataSetError as a human-readable string.
//
// Purpose:
//   Implements the standard Go error interface, formatting the error code, message, and cause.
//
// Where it is used:
//   - Automatically invoked whenever DataSetError is printed, logged, or serialized into response payloads.
//
// When can it be used:
//   - Can be used anywhere a standard error string is required.
func (e *DataSetError) Error() string {
	if e.CauseErr != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.CauseErr)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap retrieves the underlying causal error, if present.
//
// Purpose:
//   Enables Go standard library errors.Is and errors.As unwrapping chains.
//
// Where it is used:
//   - Used by error handling frameworks and tests to inspect root database or network errors.
//
// When can it be used:
//   - When checking if an underlying database driver error (e.g. pgconn.PgError) caused the failure.
func (e *DataSetError) Unwrap() error {
	return e.CauseErr
}

// NewError creates a new DataSetError with an initialized details map.
//
// Purpose:
//   Constructs a typed dataset domain error with a specific error code and human message.
//
// Where it is used:
//   - Called throughout dataset validator, planner, compiler, and service packages when errors occur.
//
// When can it be used:
//   - Can be used whenever an operation fails due to domain invariant violations or invalid parameters.
func NewError(code DataSetErrorCode, message string) *DataSetError {
	return &DataSetError{
		Code:    code,
		Message: message,
		Details: make(map[string]any),
	}
}

// NewErrorf creates a new formatted DataSetError.
//
// Purpose:
//   Constructs a typed dataset error with a printf-style formatted message.
//
// Where it is used:
//   - Used across validator, compiler, and planner when dynamic details need to be embedded in messages.
//
// When can it be used:
//   - Can be used when creating descriptive error messages with format specifiers (e.g. table names, field names).
func NewErrorf(code DataSetErrorCode, format string, args ...any) *DataSetError {
	return &DataSetError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Details: make(map[string]any),
	}
}

// WrapError wraps an existing underlying error with a typed DataSetError code.
//
// Purpose:
//   Preserves low-level root causes while attaching high-level dataset domain context and error codes.
//
// Where it is used:
//   - Called when database execution queries, JSON unmarshaling, or driver operations return unexpected errors.
//
// When can it be used:
//   - When intercepting external errors from adapters or SQL drivers and converting them into domain errors.
func WrapError(code DataSetErrorCode, message string, cause error) *DataSetError {
	return &DataSetError{
		Code:     code,
		Message:  message,
		CauseErr: cause,
		Details:  make(map[string]any),
	}
}

// WithDetail attaches debugging context metadata to the error without exposing credentials.
//
// Purpose:
//   Enriches the error object with diagnostic key-value pairs (e.g., column names, stage names).
//
// Where it is used:
//   - Called by planners and compilers to record failing AST nodes or compilation phases.
//
// When can it be used:
//   - Can be used before returning an error to append contextual parameters for upstream logging.
func (e *DataSetError) WithDetail(key string, val any) *DataSetError {
	if e.Details == nil {
		e.Details = make(map[string]any)
	}
	e.Details[key] = val
	return e
}
