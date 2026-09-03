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

func (e *DataSetError) Error() string {
	if e.CauseErr != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.CauseErr)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *DataSetError) Unwrap() error {
	return e.CauseErr
}

// NewError creates a new DataSetError.
func NewError(code DataSetErrorCode, message string) *DataSetError {
	return &DataSetError{
		Code:    code,
		Message: message,
		Details: make(map[string]any),
	}
}

// NewErrorf creates a new formatted DataSetError.
func NewErrorf(code DataSetErrorCode, format string, args ...any) *DataSetError {
	return &DataSetError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Details: make(map[string]any),
	}
}

// WrapError wraps an existing underlying error with a typed DataSetError code.
func WrapError(code DataSetErrorCode, message string, cause error) *DataSetError {
	return &DataSetError{
		Code:     code,
		Message:  message,
		CauseErr: cause,
		Details:  make(map[string]any),
	}
}

// WithDetail attaches debugging context metadata to the error without exposing credentials.
func (e *DataSetError) WithDetail(key string, val any) *DataSetError {
	if e.Details == nil {
		e.Details = make(map[string]any)
	}
	e.Details[key] = val
	return e
}
