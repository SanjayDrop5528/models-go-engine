// Package domain provides data transfer objects, domain models, and core types
// for Dataset Studio query definitions, custom columns, joins, aggregations,
// filter parameters, and execution modes.
//
// Usage:
// This package models the abstract syntax representation of datasets before compilation.
// A DataSet struct captures the root collection/table, joins, custom mathematical or
// string calculations, grouping rules, and dynamic filter parameters. It is consumed by
// DatasetPlanner and adapter compilers (Postgres, MySQL, MongoDB, In-Memory) to produce
// production-ready executable pipelines or database routines (Stored Procedures / Functions).
package domain

import (
	"encoding/json"
	"fmt"
	"strings"
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
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	ReferenceName     string           `json:"reference_name"`
	Driver            string           `json:"driver"` // "postgres", "mysql", "mongodb", "memory"
	ConversationID    string           `json:"conversation_id,omitempty"`
	BaseCollection    BaseCollection   `json:"base_collection"`
	JoinCollections   []JoinCollection `json:"join_collections,omitempty"`
	CustomColumns     []CustomColumn   `json:"custom_columns,omitempty"`
	GroupByFields     []GroupByField   `json:"group_by_fields,omitempty"`
	SchematicTable    []SchematicEntry `json:"schematic_table,omitempty"`
	Filter            []any            `json:"filter,omitempty"`
	Sort              []any            `json:"sort,omitempty"`
	FilterParams      []FilterParam    `json:"filter_params,omitempty"`
	SelectedList      []SelectedField  `json:"selected_list,omitempty"`
	SaveMode          SaveMode         `json:"save_mode"`
	Pipeline          string           `json:"pipeline,omitempty"`
	ReferencePipeline string           `json:"reference_pipeline,omitempty"`
	Status            string           `json:"status"` // "ACTIVE", "DRAFT", "ARCHIVED"
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
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
	Schema     string `json:"schema,omitempty"`
	Collection string `json:"collection"`
	Filter     any    `json:"filter,omitempty"`
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
	Schema              string   `json:"schema,omitempty"`
	FromCollection      string   `json:"fromCollection"`
	FromCollectionField string   `json:"fromCollectionField"`
	ToCollection        string   `json:"toCollection"`
	ToCollectionField   string   `json:"toCollectionField"`
	NamedAs             string   `json:"namedAs"`
	JoinType            JoinType `json:"joinType,omitempty"` // Default is LEFT
	ConvertToString     bool     `json:"convert_To_String,omitempty"`
	CastMode            string   `json:"cast_mode,omitempty"` // "BOTH", "FROM_ONLY", "TO_ONLY"
	FromFieldDataType   string   `json:"fromFieldDataType,omitempty"`
	ToFieldDataType     string   `json:"toFieldDataType,omitempty"`
	RelationshipOp      string   `json:"relationshipOp,omitempty"` // "=", "ANY", "&&", "@>", "<@", "?", "EXISTS", etc.
	Filter              any      `json:"filter,omitempty"`         // Join filter applied on the ON clause
	Cardinality         string   `json:"cardinality,omitempty"`
	OutputFormat        string   `json:"outputFormat,omitempty"`
	SourceJSONPath      []string `json:"source_json_path,omitempty"`
	TargetJSONPath      []string `json:"target_json_path,omitempty"`
}

// DataSetCustomField defines a field reference within a custom column expression.
type DataSetCustomField struct {
	Name         string `json:"name"`
	FieldName    string `json:"fieldName"`
	ParentSchema string `json:"parentSchema,omitempty"`
	TableName    string `json:"tableName"`
	Type         string `json:"type"`
	IsLiteral    bool   `json:"isLiteral,omitempty"`
	Value        string `json:"value,omitempty"`
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
	Paramvalue    interface{} `json:"paramValue,omitempty"` // Runtime value or value override
	Required      bool        `json:"required"`
}

// ExecuteRequest defines the standardized runtime execution request payload.
//
// Payload structure:
//
//	{
//	    "start": 0,
//	    "limit": 10,
//	    "filter": { ... },
//	    "sort": { ... },
//	    "appendfilter": "last", // "first" or "last" (default: "last")
//	    "filterParams": [
//	        {
//	            "ParamName": "status",
//	            "ParamDatatype": "string",
//	            "ParamValue": "active"
//	        }
//	    ]
//	}
type ExecuteRequest struct {
	Start        int    `json:"start"`
	Limit        int    `json:"limit"`
	Filter       []any  `json:"filter,omitempty"`
	Sort         []any  `json:"sort,omitempty"`
	FilterParams any    `json:"filterParams"`
	AppendFilter string `json:"appendfilter,omitempty"` // "first" or "last", default is "last"
	UserToken    any    `json:"userToken,omitempty"`
}

// NormalizeToSlice converts raw JSON (array, map, string, or scalar) into a standardized []any slice.
func NormalizeToSlice(raw []byte) ([]any, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return []any{}, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var slice []any
		if err := json.Unmarshal(raw, &slice); err != nil {
			return nil, err
		}
		if slice == nil {
			return []any{}, nil
		}
		return slice, nil
	}
	if strings.HasPrefix(trimmed, "{") {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		if len(m) == 0 {
			return []any{}, nil
		}
		return []any{m}, nil
	}
	var s any
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	if s == nil {
		return []any{}, nil
	}
	return []any{s}, nil
}

// UnmarshalJSON unmarshals DataSet normalizing Filter and Sort into []any slices.
func (ds *DataSet) UnmarshalJSON(data []byte) error {
	type Alias DataSet
	aux := struct {
		*Alias
		RawFilter json.RawMessage `json:"filter"`
		RawSort   json.RawMessage `json:"sort"`
	}{
		Alias: (*Alias)(ds),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(aux.RawFilter) > 0 {
		fSlice, err := NormalizeToSlice(aux.RawFilter)
		if err != nil {
			return err
		}
		ds.Filter = fSlice
	} else if ds.Filter == nil {
		ds.Filter = []any{}
	}
	if len(aux.RawSort) > 0 {
		sSlice, err := NormalizeToSlice(aux.RawSort)
		if err != nil {
			return err
		}
		ds.Sort = sSlice
	} else if ds.Sort == nil {
		ds.Sort = []any{}
	}
	return nil
}

// UnmarshalJSON unmarshals BaseCollection normalizing Filter into []any or map.
func (bc *BaseCollection) UnmarshalJSON(data []byte) error {
	type Alias BaseCollection
	aux := struct {
		*Alias
		RawFilter json.RawMessage `json:"filter"`
	}{
		Alias: (*Alias)(bc),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(aux.RawFilter) > 0 {
		fSlice, err := NormalizeToSlice(aux.RawFilter)
		if err != nil {
			return err
		}
		bc.Filter = fSlice
	}
	return nil
}

// UnmarshalJSON unmarshals JoinCollection normalizing Filter into []any or map.
func (jc *JoinCollection) UnmarshalJSON(data []byte) error {
	type Alias JoinCollection
	aux := struct {
		*Alias
		RawFilter json.RawMessage `json:"filter"`
	}{
		Alias: (*Alias)(jc),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(aux.RawFilter) > 0 {
		fSlice, err := NormalizeToSlice(aux.RawFilter)
		if err != nil {
			return err
		}
		jc.Filter = fSlice
	}
	return nil
}

// GetAppendFilter returns normalized "first" or "last" (defaulting to "last").
func (r *ExecuteRequest) GetAppendFilter() string {
	if r == nil {
		return "last"
	}
	if strings.EqualFold(strings.TrimSpace(r.AppendFilter), "first") {
		return "first"
	}
	return "last"
}

// UnmarshalJSON unmarshals ExecuteRequest supporting multiple casing variants for appendfilter:
// "appendfilter", "appendFilter", "append_filter", "AppendFilter", and normalizes filter and sort to []any.
func (r *ExecuteRequest) UnmarshalJSON(data []byte) error {
	type Alias ExecuteRequest
	aux := struct {
		*Alias
		RawFilter        json.RawMessage `json:"filter"`
		RawSort          json.RawMessage `json:"sort"`
		AltAppendFilter1 string          `json:"append_filter"`
		AltAppendFilter2 string          `json:"appendFilter"`
		AltAppendFilter3 string          `json:"AppendFilter"`
		AltAppendFilter4 string          `json:"appendfilter"`
	}{
		Alias: (*Alias)(r),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(aux.RawFilter) > 0 {
		fSlice, err := NormalizeToSlice(aux.RawFilter)
		if err != nil {
			return err
		}
		r.Filter = fSlice
	} else if r.Filter == nil {
		r.Filter = []any{}
	}

	if len(aux.RawSort) > 0 {
		sSlice, err := NormalizeToSlice(aux.RawSort)
		if err != nil {
			return err
		}
		r.Sort = sSlice
	} else if r.Sort == nil {
		r.Sort = []any{}
	}

	val := aux.AltAppendFilter4
	if val == "" {
		val = aux.AltAppendFilter2
	}
	if val == "" {
		val = aux.AltAppendFilter1
	}
	if val == "" {
		val = aux.AltAppendFilter3
	}

	if strings.EqualFold(strings.TrimSpace(val), "first") {
		r.AppendFilter = "first"
	} else {
		r.AppendFilter = "last"
	}
	return nil
}

// NormalizeRuntimeParams converts filterParams (whether []FilterParam, []map[string]any, or map[string]any)
// into a standardized string-to-any map for parameter substitution and binding.
//
// Parameters:
//   - None.
//
// Returns:
//   - result1 (map[string]any): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by NormalizeRuntimeParams and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func (r *ExecuteRequest) NormalizeRuntimeParams() map[string]any {
	params := make(map[string]any)
	if r == nil || r.FilterParams == nil {
		return params
	}

	switch fp := r.FilterParams.(type) {
	case map[string]any:
		for k, v := range fp {
			params[k] = v
		}
	case map[string]string:
		for k, v := range fp {
			params[k] = v
		}
	case []any:
		for _, item := range fp {
			if m, ok := item.(map[string]any); ok {
				var name string
				var val any
				for k, v := range m {
					lowerK := strings.ToLower(strings.ReplaceAll(k, "_", ""))
					switch lowerK {
					case "paramname", "name", "field":
						name = fmt.Sprintf("%v", v)
					case "paramvalue", "value", "val":
						val = v
					}
				}
				if name != "" {
					params[name] = val
				}
			}
		}
	case []FilterParam:
		for _, p := range fp {
			if p.ParamName != "" {
				if p.Paramvalue != nil {
					params[p.ParamName] = p.Paramvalue
				} else {
					params[p.ParamName] = p.DefaultValue
				}
			}
		}
	}
	return params
}
