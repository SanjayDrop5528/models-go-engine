package operation

import (
	"github.com/SanjayDrop5528/models-go-engine/model"
	"time"
)

// OperationType defines generic operation categories.
type OperationType string

const (
	OpCreate      OperationType = "CREATE"
	OpGet         OperationType = "GET"
	OpUpdate      OperationType = "UPDATE"
	OpDelete      OperationType = "DELETE"
	OpQuery       OperationType = "QUERY"
	OpFunction    OperationType = "FUNCTION"
	OpProcedure   OperationType = "PROCEDURE"
	OpCommand     OperationType = "COMMAND"
	OpTransaction OperationType = "TRANSACTION"
	OpBatch       OperationType = "BATCH"
	OpDDL         OperationType = "DDL"
	OpCustom      OperationType = "CUSTOM"
)

// OperationParameter defines an expected parameter for an Operation.
type OperationParameter struct {
	Name         string         `json:"name"`
	DataType     model.DataType `json:"data_type"`
	Required     bool           `json:"required"`
	DefaultValue any            `json:"default_value,omitempty"`
	Description  string         `json:"description,omitempty"`
}

// OperationConfig defines metadata for executing an operation (function, procedure, command, etc.).
type OperationConfig struct {
	ID          string               `json:"id"`
	ModelID     string               `json:"model_id,omitempty"` // Optional model scope
	Name        string               `json:"name"`               // Operation lookup name
	Type        OperationType        `json:"type"`               // FUNCTION, PROCEDURE, COMMAND, etc.
	Target      string               `json:"target"`             // Target DB function/procedure/collection/statement name
	Parameters  []OperationParameter `json:"parameters"`         // Parameter definitions
	ReturnType  model.DataType       `json:"return_type,omitempty"`
	IsReadOnly  bool                 `json:"is_read_only"`
	Description string               `json:"description,omitempty"`
	Metadata    map[string]any       `json:"metadata,omitempty"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
}
