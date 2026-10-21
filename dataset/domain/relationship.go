package domain

// Relationship is a metadata-ID based relationship. Empty policies default to
// AUTO, LEFT, MANY, FLAT, STRICT, NO_MATCH and DISTINCT respectively.
// Paths contain individual JSON keys (a key containing a dot is not split).
type Relationship struct {
	SourceModelID       string                  `json:"source_model_id"`
	SourceFieldID       string                  `json:"source_field_id"`
	TargetModelID       string                  `json:"target_model_id"`
	TargetFieldID       string                  `json:"target_field_id"`
	Operator            string                  `json:"operator"`
	JoinType            string                  `json:"join_type"`
	Cardinality         string                  `json:"cardinality"`
	OutputFormat        string                  `json:"output_format"`
	ConversionPolicy    string                  `json:"conversion_policy"`
	NullPolicy          string                  `json:"null_policy"`
	SourceJSONPath      []string                `json:"source_json_path"`
	TargetJSONPath      []string                `json:"target_json_path"`
	Conditions          []RelationshipCondition `json:"conditions"`
	TargetFilters       []RelationshipFilter    `json:"target_filters,omitempty"`
	PreserveSourceOrder bool                    `json:"preserve_source_order"`
	DuplicatePolicy     string                  `json:"duplicate_policy"`
	SourceOrder         []RelationshipOrder     `json:"source_order,omitempty"`
	TargetOrder         []RelationshipOrder     `json:"target_order,omitempty"`
	TargetFields        []string                `json:"target_fields,omitempty"`
	Junction            *RelationshipJunction   `json:"junction,omitempty"`
}

// RelationshipCondition is a field comparison or a recursively grouped AND/OR
// expression. Additional top-level conditions are ANDed with the primary edge.
type RelationshipCondition struct {
	SourceFieldID    string                  `json:"source_field_id,omitempty"`
	TargetFieldID    string                  `json:"target_field_id,omitempty"`
	TargetEndFieldID string                  `json:"target_end_field_id,omitempty"`
	Operator         string                  `json:"operator,omitempty"`
	SourceJSONPath   []string                `json:"source_json_path,omitempty"`
	TargetJSONPath   []string                `json:"target_json_path,omitempty"`
	Logic            string                  `json:"logic,omitempty"`
	Conditions       []RelationshipCondition `json:"conditions,omitempty"`
}

type RelationshipFilter struct {
	FieldID  string `json:"field_id"`
	Operator string `json:"operator"` // EQ, NE, IS_NULL, IS_NOT_NULL
	Value    any    `json:"value,omitempty"`
}

type RelationshipOrder struct {
	FieldID    string `json:"field_id"`
	Descending bool   `json:"descending,omitempty"`
}

// Junction connects source -> junction -> target using two scalar EQ edges.
type RelationshipJunction struct {
	ModelID       string `json:"model_id"`
	SourceFieldID string `json:"source_field_id"`
	TargetFieldID string `json:"target_field_id"`
}
