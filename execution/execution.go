package execution

import (
	"github.com/SanjayDrop5528/models-go-engine/operation"
)

// ExecutionRequest represents a normalized, database-agnostic request to execute an operation.
type ExecutionRequest struct {
	Operation operation.OperationType `json:"operation"`
	Target    string                  `json:"target"`
	Arguments map[string]any          `json:"arguments,omitempty"`
	Options   map[string]any          `json:"options,omitempty"`
}

// ExecutionResult represents the generic outcome of an executed operation.
type ExecutionResult struct {
	Data         any            `json:"data,omitempty"`
	RowsAffected int64          `json:"rows_affected,omitempty"`
	Status       string         `json:"status"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}
