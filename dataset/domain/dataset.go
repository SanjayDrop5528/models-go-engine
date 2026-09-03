package domain

import (
	"time"
)

// SaveMode defines how the compiled dataset is saved and executed on the target database.
type SaveMode string

const (
	SaveModeProcedure SaveMode = "PROCEDURE"
	SaveModeFunction  SaveMode = "FUNCTION"
	SaveModeQuery     SaveMode = "QUERY"
)

// DataSet represents the core database-independent dynamic dataset definition.
type DataSet struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	ReferenceName     string                 `json:"reference_name"`
	Driver            string                 `json:"driver"` // "postgres", "mysql", "mongodb", "memory"
	BaseCollection    BaseCollection         `json:"base_collection"`
	JoinCollections   []JoinCollection       `json:"join_collections,omitempty"`
	CustomColumns     []CustomColumn         `json:"custom_columns,omitempty"`
	GroupByFields     []GroupByField         `json:"group_by_fields,omitempty"`
	SchematicTable    []SchematicEntry       `json:"schematic_table,omitempty"`
	Filter            map[string]interface{} `json:"filter,omitempty"`
	FilterParams      []FilterParam          `json:"filter_params,omitempty"`
	SelectedList      []SelectedField        `json:"selected_list,omitempty"`
	SaveMode          SaveMode               `json:"save_mode"`
	Pipeline          string                 `json:"pipeline,omitempty"`
	ReferencePipeline string                 `json:"reference_pipeline,omitempty"`
	Status            string                 `json:"status"` // "ACTIVE", "DRAFT", "ARCHIVED"
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

// SchematicEntry represents a typed metadata schema entry (variable, param, function/procedure signature).
type SchematicEntry struct {
	Name        string      `json:"name"`
	Value       interface{} `json:"value"`
	DataType    string      `json:"dataType"`
	Category    string      `json:"category"` // "FUNCTION", "PROCEDURE", "PARAMETER", "VARIABLE", "CUSTOM_COLUMN", "JOIN_CONFIG"
	Description string      `json:"description,omitempty"`
}

// BaseCollection defines the root collection/table for the dataset query.
type BaseCollection struct {
	Schema     string                 `json:"schema,omitempty"`
	Collection string                 `json:"collection"`
	Filter     map[string]interface{} `json:"filter,omitempty"`
}

// JoinType defines SQL join types.
type JoinType string

const (
	JoinInner JoinType = "INNER"
	JoinLeft  JoinType = "LEFT"
	JoinRight JoinType = "RIGHT"
	JoinFull  JoinType = "FULL"
)

// JoinCollection defines a relationship join with table-level join filters.
type JoinCollection struct {
	FromCollection      string                 `json:"fromCollection"`
	FromCollectionField string                 `json:"fromCollectionField"`
	ToCollection        string                 `json:"toCollection"`
	ToCollectionField   string                 `json:"toCollectionField"`
	NamedAs             string                 `json:"namedAs"`
	JoinType            JoinType               `json:"joinType,omitempty"` // Default is LEFT
	ConvertToString     bool                   `json:"convert_To_String,omitempty"`
	Filter              map[string]interface{} `json:"filter,omitempty"` // Join filter applied on the ON clause
}

// DataSetCustomField defines a field reference within a custom column expression.
type DataSetCustomField struct {
	Name         string `json:"name"`
	FieldName    string `json:"fieldName"`
	ParentSchema string `json:"parentSchema,omitempty"`
	TableName    string `json:"tableName"`
	Type         string `json:"type"`
}

// CustomColumn defines calculated/virtual fields or function applications.
type CustomColumn struct {
	CustomColumnName      string               `json:"customColumnName"`
	CustomLabelName       string               `json:"customLabelName"`
	CustomAggregateFnName string               `json:"customAggregateFnName,omitempty"` // e.g. "SUM", "AVG", "CONCAT", "ADD", etc.
	Expression            string               `json:"expression,omitempty"`            // e.g. "quantity * price"
	Fields                []DataSetCustomField `json:"fields,omitempty"`
	Type                  string               `json:"type,omitempty"`
}

// GroupByField defines fields to group by in aggregated queries.
type GroupByField struct {
	TableName    string `json:"tableName"`
	ParentSchema string `json:"parentSchema,omitempty"`
	DataType     string `json:"dataType,omitempty"`
	Name         string `json:"name"`
	FieldName    string `json:"fieldName"`
}

// SelectedField defines the projection fields for preview and results.
type SelectedField struct {
	Field      string `json:"field"`
	HeaderName string `json:"headerName"`
	DataType   string `json:"dataType,omitempty"`
}

// FilterParam defines a parameter placeholder that can be supplied at runtime.
type FilterParam struct {
	ParamName     string      `json:"paramName"`
	ParamDataType string      `json:"paramDataType"` // "string", "int", "decimal", "boolean", "date", "timestamp"
	DefaultValue  interface{} `json:"defaultValue,omitempty"`
	Required      bool        `json:"required"`
}
